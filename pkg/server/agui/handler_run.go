package agui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/server"
	toolbuiltin "github.com/saker-ai/saker/pkg/tool/builtin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	aguievents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	aguisse "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/encoding/sse"
)

const keepaliveInterval = 15 * time.Second
const slowClientWriteTimeout = 30 * time.Second

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

	if err := validateRunInput(input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": err.Error(),
			"type":    "invalid_request_error",
		}})
		return
	}

	// Load shedding: reject when at capacity.
	if max := g.deps.Options.MaxActiveStreams; max > 0 {
		g.mu.Lock()
		active := len(g.activeCancels)
		g.mu.Unlock()
		if active >= max {
			aguiLoadShedTotal.Inc()
			c.Header("Retry-After", "5")
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
				"message": "server at capacity, please retry",
				"type":    "capacity_error",
			}})
			return
		}
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

	if span := trace.SpanFromContext(c.Request.Context()); span.IsRecording() {
		span.SetAttributes(
			attribute.String("agui.thread_id", threadID),
			attribute.String("agui.run_id", runID),
			attribute.String("agui.turn_id", turnID),
			attribute.String("agui.user", identity.Username),
		)
	}

	requestID := c.GetString("requestID")
	lastEventID := parseAGUILastEventID(c)
	if lastEventID > 0 {
		aguiReconnectAttemptsTotal.Inc()
	}

	g.deps.Logger.Info("agui run request received",
		"request_id", requestID,
		"thread_id", threadID,
		"run_id", runID,
		"turn_id", turnID,
		"project_id", projectID,
		"user", identity.Username,
		"last_event_id", lastEventID,
		"message_count", len(input.Messages),
		"context_count", len(input.Context),
		"prompt_len", len(sakerReq.Prompt),
	)

	// Cancel any existing run on the same thread (mutual exclusion).
	g.cancelThreadRun(threadID, runID)

	baseCtx, timeoutCancel := context.WithTimeout(c.Request.Context(), g.effectiveTurnTimeout(input))
	defer timeoutCancel()
	ctx, finishRun, ok := g.runContext(baseCtx, runID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
			"message": "agui gateway is shutting down",
			"type":    "server_shutdown",
		}})
		return
	}
	defer func() {
		finishRun()
		// Safety net: normal and cancelled paths call clearThreadRun explicitly
		// before sending terminal events. This covers early-return error paths
		// (e.g. write failures) where no terminal event is sent.
		g.clearThreadRun(threadID, runID)
	}()

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
	rc := http.NewResponseController(w)
	writeDeadlineSupported := rc.SetWriteDeadline(time.Now().Add(slowClientWriteTimeout)) == nil

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if sc := trace.SpanContextFromContext(ctx); sc.TraceID().IsValid() {
		w.Header().Set("X-Trace-Id", sc.TraceID().String())
	}
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	flushSSE := func() {
		if writeDeadlineSupported {
			_ = rc.SetWriteDeadline(time.Now().Add(slowClientWriteTimeout))
		}
		flusher.Flush()
	}

	// SSE retry directive: instruct client to reconnect after 3s on disconnect.
	fmt.Fprintf(w, "retry: 3000\n\n")
	flushSSE()

	sseW := aguisse.NewSSEWriter().WithLogger(g.deps.Logger)
	state := newStreamState(threadID, runID)
	g.mu.Lock()
	g.liveRings[runID] = state.ring
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(g.liveRings, runID)
		g.mu.Unlock()
	}()
	filter := server.NewStreamArtifactFilter()
	var accumulated strings.Builder
	eventCounts := make(map[string]int)
	streamStart := time.Now()
	aguiActiveStreams.Inc()
	defer func() {
		aguiActiveStreams.Dec()
		aguiRunDuration.Observe(time.Since(streamStart).Seconds())
	}()

	g.deps.Logger.Info("agui sse stream opened",
		"thread_id", threadID,
		"run_id", runID,
		"turn_id", turnID,
		"project_id", projectID,
	)
	if err := writeSSEWithID(ctx, w, sseW, aguievents.NewRunStartedEvent(threadID, runID), state); err != nil {
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
	if err := writeSSEWithID(ctx, w, sseW, aguievents.NewStateSnapshotEvent(
		map[string]any{"artifacts": []any{}},
	), state); err != nil {
		g.deps.Logger.Warn("agui sse initial state snapshot failed",
			"thread_id", threadID,
			"run_id", runID,
			"error", err,
		)
		return
	}
	flushSSE()

	keepalive := time.NewTicker(keepaliveInterval)
	defer keepalive.Stop()

	for {
		select {
		case evt, ok := <-eventCh:
			if !ok {
				// Persist assistant message BEFORE finalizing (which sends RUN_FINISHED)
				// so that any connect triggered by the client after RUN_FINISHED will
				// find the complete message history in the store.
				if len(state.artifacts) > 0 {
					g.storeArtifacts(threadID, state.artifacts)
				}
				g.persistAssistantMessage(context.Background(), threadID, turnID, projectID, accumulated.String())
				if g.deps.ConversationStore != nil {
					_ = g.deps.ConversationStore.CloseTurn(context.Background(), turnID, "completed")
				}
				// Clear thread→run mapping BEFORE finalize sends RUN_FINISHED.
				// This ensures that when the client receives RUN_FINISHED and
				// immediately fires agent/connect, the server no longer considers
				// this run active — so MESSAGES_SNAPSHOT is sent with full history.
				g.clearThreadRun(threadID, runID)
				if err := state.finalize(ctx, w, sseW, filter); err != nil {
					g.deps.Logger.Warn("agui sse finalize failed",
						"thread_id", threadID,
						"run_id", runID,
						"turn_id", turnID,
						"error", err,
					)
				} else {
					flushSSE()
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
			g.deps.Logger.Debug("agui runtime event received",
				"thread_id", threadID,
				"run_id", runID,
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
				if isSlowClientErr(err) {
					aguiSlowClientDisconnects.Inc()
				}
				g.deps.Logger.Warn("agui sse translate/write failed",
					"thread_id", threadID,
					"run_id", runID,
					"turn_id", turnID,
					"event_type", evt.Type,
					"slow_client", isSlowClientErr(err),
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
					if err := writeSSEWithID(ctx, w, sseW, aguievents.NewStateDeltaEvent(delta), state); err != nil {
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
			flushSSE()

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
				if err := writeSSEWithID(ctx, w, sseW, event, state); err != nil {
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
			g.deps.Logger.Debug("agui side event sent",
				"thread_id", threadID,
				"run_id", runID,
				"event_count", len(se.events),
				"event_types", sideTypes,
			)
			flushSSE()

		case <-keepalive.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flushSSE()

		case <-ctx.Done():
			// Clear thread→run mapping before emitting RUN_ERROR for the same
			// reason as the normal path: client may reconnect immediately.
			g.clearThreadRun(threadID, runID)
			aguiErrorsTotal.WithLabelValues("cancelled").Inc()
			errCtx := context.Background()
			_ = writeSSEWithID(errCtx, w, sseW, aguievents.NewRunErrorEvent("stream cancelled",
				aguievents.WithRunID(runID), aguievents.WithErrorCode("cancelled")), state)
			flushSSE()
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
	g.artifactCache.store(threadID, arts)
}

// loadArtifacts returns cached artifacts for a thread.
func (g *Gateway) loadArtifacts(threadID string) []server.Artifact {
	return g.artifactCache.load(threadID)
}

// cancelThreadRun cancels an existing run on the same thread (mutual exclusion).
func (g *Gateway) cancelThreadRun(threadID, newRunID string) {
	g.mu.Lock()
	oldRunID, exists := g.threadRuns[threadID]
	g.threadRuns[threadID] = newRunID
	var oldCancel context.CancelFunc
	if exists && oldRunID != newRunID {
		oldCancel = g.activeCancels[oldRunID]
	}
	g.mu.Unlock()
	if oldCancel != nil {
		oldCancel()
	}
}

// clearThreadRun removes the thread→run mapping when a run finishes.
func (g *Gateway) clearThreadRun(threadID, runID string) {
	g.mu.Lock()
	if g.threadRuns[threadID] == runID {
		delete(g.threadRuns, threadID)
	}
	g.mu.Unlock()
}

// parseAGUILastEventID extracts the last event ID from a reconnection request.
func parseAGUILastEventID(c *gin.Context) int {
	raw := c.Query("last_event_id")
	if raw == "" {
		raw = c.GetHeader("Last-Event-ID")
	}
	if raw == "" {
		return 0
	}
	seq, err := strconv.Atoi(raw)
	if err != nil || seq < 0 {
		return 0
	}
	return seq
}

// isSlowClientErr returns true if the error indicates a write deadline exceeded.
func isSlowClientErr(err error) bool {
	return err != nil && errors.Is(err, os.ErrDeadlineExceeded)
}

// effectiveTurnTimeout returns the turn timeout for a run, respecting operator
// cap and optional client-requested shorter timeout via ForwardedProps.
func (g *Gateway) effectiveTurnTimeout(input aguitypes.RunAgentInput) time.Duration {
	cap := g.deps.Options.TurnTimeout
	if cap == 0 {
		cap = server.DefaultTurnTimeout
	}
	props, ok := input.ForwardedProps.(map[string]any)
	if !ok || props == nil {
		return cap
	}
	raw, ok := props["timeout_seconds"]
	if !ok {
		return cap
	}
	var seconds float64
	switch v := raw.(type) {
	case float64:
		seconds = v
	case json.Number:
		seconds, _ = v.Float64()
	default:
		return cap
	}
	if seconds <= 0 {
		return cap
	}
	requested := time.Duration(seconds * float64(time.Second))
	if requested < cap {
		return requested
	}
	return cap
}
