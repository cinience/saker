package agui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	aguievents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	aguisse "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/encoding/sse"

	"github.com/saker-ai/saker/pkg/conversation"
)

// handleConnectRoute is the REST-transport entry point for /agent/:agentId/connect.
// It reads the raw body and delegates to handleConnect.
func (g *Gateway) handleConnectRoute(c *gin.Context) {
	body, err := readBody(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "failed to read request body: " + err.Error(),
			"type":    "invalid_request_error",
		}})
		return
	}
	g.handleConnect(c, body)
}

// handleConnect implements the AG-UI /agent/connect endpoint. CopilotKit
// calls this when a threadId is set (or changes) to load existing messages.
// Response is an SSE stream: RUN_STARTED → MESSAGES_SNAPSHOT → RUN_FINISHED.
func (g *Gateway) handleConnect(c *gin.Context, body []byte) {
	var input aguitypes.RunAgentInput
	if body != nil {
		if err := json.Unmarshal(body, &input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
				"message": "invalid JSON body: " + err.Error(),
				"type":    "invalid_request_error",
			}})
			return
		}
	}

	threadID := input.ThreadID
	if threadID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "threadId is required for connect",
			"type":    "invalid_request_error",
		}})
		return
	}

	runID := input.RunID
	if runID == "" {
		runID = "run_" + uuid.New().String()
	}

	// If there's an active run on this thread, skip loading messages to avoid
	// sending a MESSAGES_SNAPSHOT that would reset the in-flight streaming state.
	g.mu.Lock()
	activeRunID := g.threadRuns[threadID]
	g.mu.Unlock()
	hasActiveRun := activeRunID != ""

	var aguiMessages []aguitypes.Message
	if !hasActiveRun && g.deps.ConversationStore != nil {
		msgs, err := g.deps.ConversationStore.GetMessages(
			c.Request.Context(), threadID, conversation.GetMessagesOpts{},
		)
		if err != nil {
			g.deps.Logger.Warn("agui connect: failed to load messages",
				"thread_id", threadID, "error", err)
		} else {
			aguiMessages = convertMessages(msgs)
		}
	}

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

	sseW := aguisse.NewSSEWriter().WithLogger(g.deps.Logger)

	if err := writeSSE(c.Request.Context(), w, sseW, aguievents.NewRunStartedEvent(threadID, runID)); err != nil {
		g.deps.Logger.Warn("agui connect: failed to write run start", "thread_id", threadID, "run_id", runID, "error", err)
		return
	}
	// Skip MESSAGES_SNAPSHOT when a run is actively streaming to avoid resetting
	// the in-flight streaming state in the client.
	if !hasActiveRun {
		if err := writeSSE(c.Request.Context(), w, sseW, aguievents.NewMessagesSnapshotEvent(aguiMessages)); err != nil {
			g.deps.Logger.Warn("agui connect: failed to write snapshot", "thread_id", threadID, "run_id", runID, "error", err)
			return
		}
	}
	// Emit state snapshot with cached artifacts from previous runs.
	var artifactState []any
	if cached := g.loadArtifacts(threadID); len(cached) > 0 {
		for _, a := range cached {
			artifactState = append(artifactState, map[string]string{
				"type": a.Type, "url": a.URL, "name": a.Name,
			})
		}
	}
	if artifactState == nil {
		artifactState = []any{}
	}
	if err := writeSSE(c.Request.Context(), w, sseW, aguievents.NewStateSnapshotEvent(
		map[string]any{"artifacts": artifactState},
	)); err != nil {
		g.deps.Logger.Warn("agui connect: failed to write state snapshot", "thread_id", threadID, "run_id", runID, "error", err)
		return
	}
	if err := writeSSE(c.Request.Context(), w, sseW, aguievents.NewRunFinishedEvent(threadID, runID)); err != nil {
		g.deps.Logger.Warn("agui connect: failed to write run finished", "thread_id", threadID, "run_id", runID, "error", err)
		return
	}
	flusher.Flush()
}

// convertMessages translates conversation.Message rows into AG-UI typed
// messages suitable for a MESSAGES_SNAPSHOT event. Supports all AG-UI roles:
// user, assistant, system, tool, developer, reasoning, activity.
func convertMessages(msgs []conversation.Message) []aguitypes.Message {
	out := make([]aguitypes.Message, 0, len(msgs))
	for i := range msgs {
		m := &msgs[i]
		if shouldSkipHistoryMessage(m, out) {
			continue
		}
		am := aguitypes.Message{
			ID:         strconv.FormatInt(m.ID, 10),
			Role:       aguitypes.Role(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			am.ToolCalls = convertToolCalls(m.ToolCalls)
		}
		out = append(out, am)
	}
	return out
}

func shouldSkipHistoryMessage(m *conversation.Message, out []aguitypes.Message) bool {
	if m == nil {
		return true
	}
	content := strings.TrimSpace(m.Content)
	if m.Role == "assistant" && content == "" && len(m.ToolCalls) == 0 {
		return true
	}
	if m.Role == "user" && strings.HasPrefix(content, "[System] You asked questions in plain text.") {
		return true
	}
	if len(out) == 0 {
		return false
	}
	prev := out[len(out)-1]
	return string(prev.Role) == m.Role &&
		strings.TrimSpace(fmt.Sprint(prev.Content)) == content &&
		prev.ToolCallID == m.ToolCallID &&
		len(prev.ToolCalls) == 0
}

// convertToolCalls deserializes the stored [{id, name, arguments}] JSON
// array into AG-UI typed ToolCall structs.
func convertToolCalls(raw json.RawMessage) []aguitypes.ToolCall {
	var stored []struct {
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil
	}
	out := make([]aguitypes.ToolCall, len(stored))
	for i, tc := range stored {
		args := string(tc.Arguments)
		if args == "" || args == "null" {
			args = "{}"
		}
		out[i] = aguitypes.ToolCall{
			ID:   tc.ID,
			Type: aguitypes.ToolCallTypeFunction,
			Function: aguitypes.FunctionCall{
				Name:      tc.Name,
				Arguments: args,
			},
		}
	}
	return out
}

// formatThreadResponse converts a conversation.Thread to the JSON shape
// CopilotKit v2 expects: {id, name, createdAt, updatedAt}.
func formatThreadResponse(t *conversation.Thread) gin.H {
	return gin.H{
		"id":        t.ID,
		"name":      t.Title,
		"title":     t.Title,
		"createdAt": t.CreatedAt.Format(time.RFC3339),
		"updatedAt": t.UpdatedAt.Format(time.RFC3339),
	}
}
