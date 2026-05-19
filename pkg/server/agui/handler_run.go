package agui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/server"
	toolbuiltin "github.com/saker-ai/saker/pkg/tool/builtin"

	aguievents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	aguisse "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/encoding/sse"
)

const keepaliveInterval = 15 * time.Second

// handleRun implements POST /v1/agents/run — the main AG-UI streaming endpoint.
func (g *Gateway) handleRun(c *gin.Context) {
	body, err := readBody(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "failed to read request body: " + err.Error(),
			"type":    "invalid_request_error",
		}})
		return
	}

	inner, envelopeMethod := aguiUnwrapEnvelope(c, body)
	switch envelopeMethod {
	case "info":
		g.handleInfo(c)
		return
	case "capabilities":
		g.handleCapabilities(c)
		return
	case "threads":
		g.handleThreads(c)
		return
	case "agent/stop":
		g.handleStop(c)
		return
	case "agent/connect":
		g.handleConnect(c, inner)
		return
	}
	if inner != nil {
		body = inner
	}

	var input aguitypes.RunAgentInput
	if err := json.Unmarshal(body, &input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "invalid JSON body: " + err.Error(),
			"type":    "invalid_request_error",
		}})
		return
	}

	if len(input.Messages) == 0 {
		g.handleInfo(c)
		return
	}

	threadID := input.ThreadID
	if threadID == "" {
		threadID = "thread_" + uuid.New().String()
	}
	runID := input.RunID
	if runID == "" {
		runID = "run_" + uuid.New().String()
	}

	identity := identityFromContext(c.Request.Context())
	sakerReq := messagesToRequest(input, identity)

	projectID := identity.ProjectID
	if projectID == "" {
		projectID = "default"
	}

	g.ensureThread(c.Request.Context(), threadID, identity)

	turnID := runID
	if g.deps.ConversationStore != nil {
		if tid, err := g.deps.ConversationStore.OpenTurn(c.Request.Context(), threadID, ""); err == nil {
			turnID = tid
		}
	}

	g.persistUserMessage(c.Request.Context(), threadID, turnID, projectID, sakerReq.Prompt)

	g.deps.Logger.Info("agui run request received",
		"thread_id", threadID,
		"run_id", runID,
		"turn_id", turnID,
		"project_id", projectID,
		"user", identity.Username,
		"message_count", len(input.Messages),
		"context_count", len(input.Context),
		"prompt_len", len(sakerReq.Prompt),
	)

	baseCtx, timeoutCancel := context.WithTimeout(c.Request.Context(), server.DefaultTurnTimeout)
	defer timeoutCancel()
	ctx, finishRun, ok := g.runContext(baseCtx, runID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
			"message": "agui gateway is shutting down",
			"type":    "server_shutdown",
		}})
		return
	}
	defer finishRun()

	sideCh := make(chan sideEvent, 8)
	ctx = toolbuiltin.WithAskQuestionFunc(ctx, g.makeAskQuestionHandler(runID, sideCh))

	sakerReq.Metadata = mergeMetadata(sakerReq.Metadata, map[string]any{
		"_agui_run_id":             runID,
		"_agui_permission_handler": g.makePermissionHandler(runID, sideCh),
	})

	eventCh, err := g.deps.Runtime.RunStream(ctx, sakerReq)
	if err != nil {
		g.deps.Logger.Error("agui runtime failed to start",
			"thread_id", threadID,
			"run_id", runID,
			"turn_id", turnID,
			"error", err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "failed to start run: " + err.Error(),
			"type":    "server_error",
		}})
		return
	}
	g.deps.Logger.Info("agui runtime stream started",
		"thread_id", threadID,
		"run_id", runID,
		"turn_id", turnID,
	)

	g.streamSSE(c, ctx, eventCh, sideCh, threadID, runID, turnID, projectID)
}

// streamSSE writes the AG-UI event stream to the client as SSE.
func (g *Gateway) streamSSE(c *gin.Context, ctx context.Context, eventCh <-chan api.StreamEvent, sideCh <-chan sideEvent, threadID, runID, turnID, projectID string) {
	w := c.Writer
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		g.deps.Logger.Debug("agui sse write deadline not adjustable",
			"thread_id", threadID,
			"run_id", runID,
			"error", err,
		)
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	flusher.Flush()

	sseW := aguisse.NewSSEWriter().WithLogger(g.deps.Logger)
	state := newStreamState(threadID, runID)
	filter := server.NewStreamArtifactFilter()
	var accumulated strings.Builder
	eventCounts := make(map[string]int)

	g.deps.Logger.Info("agui sse stream opened",
		"thread_id", threadID,
		"run_id", runID,
		"turn_id", turnID,
		"project_id", projectID,
	)
	if err := writeSSE(ctx, w, sseW, aguievents.NewRunStartedEvent(threadID, runID)); err != nil {
		g.deps.Logger.Warn("agui sse write failed",
			"thread_id", threadID,
			"run_id", runID,
			"event_type", "RUN_STARTED",
			"error", err,
		)
		return
	}
	// Establish the shared state schema so CopilotKit's useCoAgent
	// knows that state.artifacts is an array before StateDeltaEvents arrive.
	if err := writeSSE(ctx, w, sseW, aguievents.NewStateSnapshotEvent(
		map[string]any{"artifacts": []any{}},
	)); err != nil {
		g.deps.Logger.Warn("agui sse initial state snapshot failed",
			"thread_id", threadID,
			"run_id", runID,
			"error", err,
		)
		return
	}
	flusher.Flush()

	keepalive := time.NewTicker(keepaliveInterval)
	defer keepalive.Stop()

	for {
		select {
		case evt, ok := <-eventCh:
			if !ok {
				if err := state.finalize(ctx, w, sseW, filter); err != nil {
					g.deps.Logger.Warn("agui sse finalize failed",
						"thread_id", threadID,
						"run_id", runID,
						"turn_id", turnID,
						"error", err,
					)
				} else {
					flusher.Flush()
				}
				if len(state.artifacts) > 0 {
					g.storeArtifacts(threadID, state.artifacts)
				}
				g.persistAssistantMessage(context.Background(), threadID, turnID, projectID, accumulated.String())
				if g.deps.ConversationStore != nil {
					_ = g.deps.ConversationStore.CloseTurn(context.Background(), turnID, "completed")
				}
				g.deps.Logger.Info("agui runtime stream completed",
					"thread_id", threadID,
					"run_id", runID,
					"turn_id", turnID,
					"assistant_len", accumulated.Len(),
					"event_counts", eventCounts,
				)
				return
			}
			eventCounts[string(evt.Type)]++
			g.deps.Logger.Info("agui runtime event received",
				"thread_id", threadID,
				"run_id", runID,
				"turn_id", turnID,
				"event_type", evt.Type,
				"tool_use_id", evt.ToolUseID,
				"tool_name", evt.Name,
				"delta_len", deltaTextLen(evt),
			)
			if evt.Type == api.EventContentBlockDelta && evt.Delta != nil && evt.Delta.Text != "" {
				accumulated.WriteString(evt.Delta.Text)
			}
			newArtifacts, err := state.translateEvent(ctx, w, sseW, evt, filter)
			if err != nil {
				g.deps.Logger.Warn("agui sse translate/write failed",
					"thread_id", threadID,
					"run_id", runID,
					"turn_id", turnID,
					"event_type", evt.Type,
					"error", err,
				)
				return
			}
			if len(newArtifacts) > 0 {
				cacher := g.deps.MediaCacher
				for _, a := range newArtifacts {
					cached := a
					if cacher != nil && (strings.HasPrefix(a.URL, "http://") || strings.HasPrefix(a.URL, "https://")) {
						cached = cacher.CacheArtifactMedia(ctx, a)
					}
					delta := []aguievents.JSONPatchOperation{
						{
							Op:   "add",
							Path: "/artifacts/-",
							Value: map[string]string{
								"type": cached.Type,
								"url":  cached.URL,
								"name": cached.Name,
							},
						},
					}
					if err := writeSSE(ctx, w, sseW, aguievents.NewStateDeltaEvent(delta)); err != nil {
						g.deps.Logger.Warn("agui sse state delta write failed",
							"thread_id", threadID,
							"run_id", runID,
							"artifact_url", cached.URL,
							"error", err,
						)
						return
					}
				}
			}
			flusher.Flush()

		case se := <-sideCh:
			if len(se.events) == 0 {
				continue
			}
			sideTypes := make([]string, 0, len(se.events))
			for _, event := range se.events {
				if event == nil {
					continue
				}
				sideTypes = append(sideTypes, string(event.Type()))
				if err := writeSSE(ctx, w, sseW, event); err != nil {
					g.deps.Logger.Warn("agui side-event write failed",
						"thread_id", threadID,
						"run_id", runID,
						"turn_id", turnID,
						"event_type", event.Type(),
						"error", err,
					)
					return
				}
			}
			g.deps.Logger.Info("agui side event sent",
				"thread_id", threadID,
				"run_id", runID,
				"turn_id", turnID,
				"event_count", len(se.events),
				"event_types", sideTypes,
			)
			flusher.Flush()

		case <-keepalive.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()

		case <-ctx.Done():
			g.deps.Logger.Info("agui runtime stream cancelled",
				"thread_id", threadID,
				"run_id", runID,
				"turn_id", turnID,
				"error", ctx.Err(),
				"assistant_len", accumulated.Len(),
				"event_counts", eventCounts,
			)
			return
		}
	}
}

func deltaTextLen(evt api.StreamEvent) int {
	if evt.Delta == nil {
		return 0
	}
	return len(evt.Delta.Text)
}

func mergeMetadata(base, extra map[string]any) map[string]any {
	if base == nil {
		return extra
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}

// storeArtifacts appends artifacts to the per-thread cache for replay on connect.
func (g *Gateway) storeArtifacts(threadID string, arts []server.Artifact) {
	existing, _ := g.artifactCache.Load(threadID)
	var all []server.Artifact
	if existing != nil {
		all = existing.([]server.Artifact)
	}
	all = append(all, arts...)
	g.artifactCache.Store(threadID, all)
}

// loadArtifacts returns cached artifacts for a thread.
func (g *Gateway) loadArtifacts(threadID string) []server.Artifact {
	v, ok := g.artifactCache.Load(threadID)
	if !ok {
		return nil
	}
	return v.([]server.Artifact)
}
