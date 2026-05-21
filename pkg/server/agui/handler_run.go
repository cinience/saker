package agui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/server"
	toolbuiltin "github.com/saker-ai/saker/pkg/tool/builtin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

const keepaliveInterval = 15 * time.Second
const slowClientWriteTimeout = 30 * time.Second

// handleRun implements POST /v1/agents/run — the main AG-UI streaming endpoint.
// It routes to either a new run or a reconnect to an existing session.
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

	lastEventID := parseAGUILastEventID(c)
	runID := input.RunID
	if runID == "" {
		runID = "run_" + uuid.New().String()
	}

	// Reconnect path: client is resuming an existing session.
	if lastEventID > 0 {
		aguiReconnectAttemptsTotal.Inc()
		session := g.sessions.get(runID)
		if session != nil {
			g.handleReconnect(c, session, lastEventID)
			return
		}
		// Session not found — it expired or was never created.
		c.JSON(http.StatusGone, gin.H{"error": gin.H{
			"message": "session not found or expired, please start a new run",
			"type":    "session_expired",
		}})
		return
	}

	// New run path.
	g.handleNewRun(c, input, runID)
}

// handleNewRun creates a new session and starts the runtime.
func (g *Gateway) handleNewRun(c *gin.Context, input aguitypes.RunAgentInput, runID string) {
	// Load shedding: reject when at capacity.
	if max := g.deps.Options.MaxActiveStreams; max > 0 {
		if g.sessions.count() >= max {
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

	identity := identityFromContext(c.Request.Context())
	sakerReq := messagesToRequest(input, identity)

	projectID := identity.ProjectID
	if projectID == "" {
		projectID = "default"
	}
	turnID := runID

	if span := trace.SpanFromContext(c.Request.Context()); span.IsRecording() {
		span.SetAttributes(
			attribute.String("agui.thread_id", threadID),
			attribute.String("agui.run_id", runID),
			attribute.String("agui.turn_id", turnID),
			attribute.String("agui.user", identity.Username),
		)
	}

	requestID := c.GetString("requestID")
	g.deps.Logger.Info("agui new run request",
		"request_id", requestID,
		"thread_id", threadID,
		"run_id", runID,
		"turn_id", turnID,
		"project_id", projectID,
		"user", identity.Username,
		"message_count", len(input.Messages),
		"prompt_len", len(sakerReq.Prompt),
	)

	// Cancel any existing run on the same thread (mutual exclusion).
	g.cancelThreadRun(threadID, runID)

	// Create runtime context decoupled from HTTP connection.
	// The runtime survives client disconnects so the session can be resumed.
	// Chain: WithoutCancel(reqCtx) → WithTimeout → runContext(WithCancel)
	// cancelThreadRun uses activeCancels to cancel the runContext child.
	// session.runtimeCancel uses baseCancel which cancels the timeout parent.
	baseCtx, baseCancel := context.WithTimeout(
		context.WithoutCancel(c.Request.Context()),
		g.effectiveTurnTimeout(input),
	)

	runtimeCtx, finishRun, ok := g.runContext(baseCtx, runID)
	if !ok {
		baseCancel()
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
			"message": "agui gateway is shutting down",
			"type":    "server_shutdown",
		}})
		return
	}

	sideCh := make(chan sideEvent, 8)
	runtimeCtx = toolbuiltin.WithAskQuestionFunc(runtimeCtx, g.makeAskQuestionHandler(runID, sideCh))
	if g.deps.CtxEnricher != nil {
		runtimeCtx = g.deps.CtxEnricher(runtimeCtx)
	}

	sakerReq.Metadata = mergeMetadata(sakerReq.Metadata, map[string]any{
		"_agui_run_id":             runID,
		"_agui_permission_handler": g.makePermissionHandler(runID, sideCh),
	})

	// Per-session dynamic MCP servers from client ForwardedProps.
	var mcpReg *sessionMCPRegistry
	if props, ok := input.ForwardedProps.(map[string]any); ok && props != nil {
		if mcpServers, parseErr := extractMCPServers(props); parseErr != nil {
			baseCancel()
			finishRun()
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
				"message": "invalid mcp_servers: " + parseErr.Error(),
				"type":    "invalid_request_error",
			}})
			return
		} else if len(mcpServers) > 0 {
			if valErr := validateMCPAllowList(mcpServers, g.deps.Options.AllowedMCPPatterns); valErr != nil {
				baseCancel()
				finishRun()
				c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
					"message": valErr.Error(),
					"type":    "permission_error",
				}})
				return
			}
			if secErr := validateMCPSecurity(mcpServers, g.deps.Options); secErr != nil {
				baseCancel()
				finishRun()
				c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
					"message": secErr.Error(),
					"type":    "permission_error",
				}})
				return
			}
			// Try to reuse a cached registry for this thread (cross-turn reuse).
			mcpReg = g.mcpCache.get(threadID)
			if mcpReg != nil {
				aguiMCPCacheHits.Inc()
			} else {
				aguiMCPCacheMisses.Inc()
				reg := newSessionMCPRegistry(g.deps.Logger)
				if t := g.deps.Options.MCPConnectTimeout; t > 0 {
					reg.connectTimeout = t
				}
				mcpReg = reg
			}
			if connErr := mcpReg.EnsureServers(runtimeCtx, mcpServers, nil); connErr != nil {
				mcpReg.Close()
				g.mcpCache.remove(threadID)
				baseCancel()
				finishRun()
				c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
					"message": "failed to connect MCP servers: " + connErr.Error(),
					"type":    "mcp_connection_error",
				}})
				return
			}
			sakerReq.DynamicExecutor = mcpReg
		}
	}

	eventCh, err := g.deps.Runtime.RunStream(runtimeCtx, sakerReq)
	if err != nil {
		baseCancel()
		finishRun()
		g.deps.Logger.Error("agui runtime failed to start",
			"thread_id", threadID, "run_id", runID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "failed to start run: " + err.Error(),
			"type":    "server_error",
		}})
		return
	}

	// Create session and start pump.
	// baseCancel cancels the parent context, which propagates to runtimeCtx.
	session := newRunSession(g, runID, threadID, turnID, projectID, eventCh, sideCh, runtimeCtx, baseCancel)
	session.mcpRegistry = mcpReg
	g.sessions.register(session)

	go session.pump(finishRun)

	// Attach HTTP client to session.
	g.attachClientToSession(c, session)
}

// handleReconnect resumes an existing session by replaying buffered events.
func (g *Gateway) handleReconnect(c *gin.Context, session *runSession, lastEventID int) {
	requestID := c.GetString("requestID")
	g.deps.Logger.Info("agui reconnect attempt",
		"request_id", requestID,
		"run_id", session.runID,
		"thread_id", session.threadID,
		"last_event_id", lastEventID,
	)

	// Set up SSE response.
	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	fmt.Fprintf(w, "retry: 3000\n\n")
	flusher.Flush()

	client := &attachedClient{
		writer:  w,
		flusher: flusher,
		doneCh:  make(chan struct{}),
	}

	if err := session.replayAndAttach(client, lastEventID); err != nil {
		// Ring overflow — client must start a new run.
		aguiReconnectOverflowTotal.Inc()
		g.deps.Logger.Warn("agui reconnect failed: ring overflow",
			"run_id", session.runID, "last_event_id", lastEventID, "error", err)
		// Already started writing SSE, send error as event.
		fmt.Fprintf(w, "event: error\ndata: {\"message\":\"reconnect failed: %s\",\"code\":\"ring_overflow\"}\n\n", err.Error())
		flusher.Flush()
		return
	}

	g.deps.Logger.Info("agui reconnect succeeded",
		"run_id", session.runID,
		"thread_id", session.threadID,
		"last_event_id", lastEventID,
	)

	// Block until client disconnects or session finishes.
	g.waitForClientDone(c, session, client)
}

// attachClientToSession sets up SSE headers and attaches the client to the session.
func (g *Gateway) attachClientToSession(c *gin.Context, session *runSession) {
	w := c.Writer
	rc := http.NewResponseController(w)
	writeDeadlineSupported := rc.SetWriteDeadline(time.Now().Add(slowClientWriteTimeout)) == nil

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if sc := trace.SpanContextFromContext(c.Request.Context()); sc.TraceID().IsValid() {
		w.Header().Set("X-Trace-Id", sc.TraceID().String())
	}
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	// SSE retry directive: instruct client to reconnect after 3s on disconnect.
	fmt.Fprintf(w, "retry: 3000\n\n")
	flusher.Flush()

	client := &attachedClient{
		writer:  w,
		flusher: flusher,
		doneCh:  make(chan struct{}),
	}

	// Apply write deadline refreshing via a wrapper.
	if writeDeadlineSupported {
		client.writer = &deadlineWriter{w: w, rc: rc, timeout: slowClientWriteTimeout}
	}

	session.attach(client)

	g.waitForClientDone(c, session, client)
}

// waitForClientDone blocks until the HTTP client disconnects or the session finishes.
func (g *Gateway) waitForClientDone(c *gin.Context, session *runSession, client *attachedClient) {
	select {
	case <-c.Request.Context().Done():
		// Client disconnected — detach but don't cancel runtime.
		session.detach()
	case <-session.doneCh:
		// Session finished (pump exited).
	}
}

// deadlineWriter wraps an io.Writer and refreshes the write deadline on each write.
type deadlineWriter struct {
	w       io.Writer
	rc      *http.ResponseController
	timeout time.Duration
}

func (d *deadlineWriter) Write(p []byte) (int, error) {
	_ = d.rc.SetWriteDeadline(time.Now().Add(d.timeout))
	return d.w.Write(p)
}

// --- Helper functions (unchanged) ---

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
