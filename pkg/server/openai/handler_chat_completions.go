package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/saker-ai/saker/pkg/model"
	"github.com/saker-ai/saker/pkg/runhub"
	"github.com/saker-ai/saker/pkg/server"
	toolbuiltin "github.com/saker-ai/saker/pkg/tool/builtin"
	"go.opentelemetry.io/otel/trace"
)

// keepaliveInterval is how often we emit an SSE comment frame to keep
// proxies (cloudflare, nginx) from idling out the connection. 15 s is
// well below typical 60 s defaults.
const keepaliveInterval = 15 * time.Second

// handleChatCompletions implements POST /v1/chat/completions.
//
// Flow:
//  1. Read + decode body.
//  2. Validate model / messages / extra_body.
//  3. Resolve identity from authMiddleware → tag the saker request.
//  4. MessagesToRequest folds OpenAI messages → saker Request (Ephemeral=true).
//  5. Register hub.Run with cancel func tied to producer goroutine.
//  6. Spawn producer that drains Runtime.RunStream → translates → publishes.
//  7. Consumer (this goroutine) writes SSE (stream=true) or aggregates a
//     single chat.completion JSON (stream=false).
func (g *Gateway) handleChatCompletions(c *gin.Context) {
	maxBody := g.deps.Options.MaxRequestBodyBytes
	if maxBody <= 0 {
		maxBody = 10 * 1024 * 1024
	}
	rawBody, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBody+1))
	if err != nil {
		InvalidRequest(c, "failed to read request body: "+err.Error())
		return
	}
	if int64(len(rawBody)) > maxBody {
		InvalidRequest(c, fmt.Sprintf("request body exceeds %d bytes", maxBody))
		return
	}

	var req ChatRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		InvalidRequest(c, "invalid JSON body: "+err.Error())
		return
	}

	if strings.TrimSpace(req.Model) == "" {
		InvalidRequestField(c, "model", "field 'model' is required")
		return
	}
	if len(req.Messages) == 0 {
		InvalidRequestField(c, "messages", "field 'messages' must contain at least one message")
		return
	}
	if req.N > 1 {
		InvalidRequestField(c, "n", "saker gateway supports n=1 only")
		return
	}

	extra, err := ParseExtraBody(req.ExtraBody)
	if err != nil {
		InvalidRequest(c, err.Error())
		return
	}

	tier := ResolveModelTier(req.Model)
	sakerReq, err := MessagesToRequest(c.Request.Context(), req.Messages, extra, tier)
	if err != nil {
		InvalidRequest(c, err.Error())
		return
	}

	// Inject client-provided tools as ExtraTools so the LLM is aware of
	// them (e.g. HITL tools like ask_user_question, confirmAction). These
	// are NOT registered in saker's tool executor — they're passthrough.
	if len(req.Tools) > 0 {
		for _, ct := range req.Tools {
			sakerReq.ExtraTools = append(sakerReq.ExtraTools, model.ToolDefinition{
				Name:        ct.Function.Name,
				Description: ct.Function.Description,
				Parameters:  ct.Function.Parameters,
			})
		}
	}

	// Forward the OpenAI standard sampling knobs (temperature, top_p,
	// max_tokens, stop, seed, tool_choice, parallel_tool_calls) onto the
	// agent runtime. Provider adapters that don't consume a given field
	// silently ignore it, so this is safe to attach unconditionally.
	sakerReq.ModelOverrides = buildModelOverrides(req)

	identity := IdentityFromContext(c.Request.Context())
	if identity.Username != "" {
		sakerReq.User = identity.Username
	}
	if identity.ProjectID != "" {
		sakerReq.ProjectID = identity.ProjectID
	}
	tenantID := identity.APIKeyID
	if tenantID == "" {
		tenantID = identity.Username
	}

	// Resume path: if session has a pending ask_user_question and the client
	// is submitting a tool response, resume the paused run instead of starting
	// a new one.
	if extra.SessionID != "" && g.pendingAsks != nil {
		if pa := g.pendingAsks.LookupBySession(extra.SessionID, tenantID); pa != nil {
			lastMsg := req.Messages[len(req.Messages)-1]
			if lastMsg.Role == "tool" {
				g.handleResumeToolCall(c, pa, req, extra)
				return
			}
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{
				"message":              "session has a pending tool_call awaiting response",
				"type":                 "invalid_request_error",
				"code":                 "session_awaiting_tool_response",
				"param":                "extra_body.session_id",
				"pending_run_id":       pa.RunID,
				"pending_tool_call_id": pa.ToolCallID,
			}})
			return
		}
	}

	// Expose the effective thread/session ID so SDK clients can track it.
	if sakerReq.SessionID != "" {
		c.Writer.Header().Set("X-Saker-Thread-Id", sakerReq.SessionID)
	}

	expiresAfter := g.deps.Options.ExpiresAfter()
	if extra.ExpiresAfterSeconds > 0 {
		expiresAfter = time.Duration(extra.ExpiresAfterSeconds) * time.Second
	}
	turnTimeout := server.DefaultTurnTimeout
	if expiresAfter > turnTimeout {
		expiresAfter = turnTimeout
	}

	// Producer ctx is detached from the client unless cancel_on_disconnect
	// is set. The detached path lets the run keep going (and stay
	// reconnectable in P1) after the client closes the SSE socket.
	var (
		producerCtx    context.Context
		producerCancel context.CancelFunc
	)
	if extra.EffectiveCancelOnDisconnect() {
		producerCtx, producerCancel = context.WithTimeout(c.Request.Context(), turnTimeout)
	} else {
		producerCtx, producerCancel = context.WithTimeout(context.Background(), turnTimeout)
	}

	hubRun, err := g.hub.Create(runhub.CreateOptions{
		SessionID: sakerReq.SessionID,
		TenantID:  tenantID,
		ExpiresAt: time.Now().Add(expiresAfter),
		Cancel:    producerCancel,
	})
	if err != nil {
		producerCancel()
		if errors.Is(err, runhub.ErrCapacity) {
			RateLimited(c, "too many in-flight runs; try again later")
			return
		}
		ServerError(c, "failed to register run: "+err.Error())
		return
	}

	hubRun.SetStatus(runhub.RunStatusInProgress)

	// Surface the run id so clients can correlate against server logs and
	// (in P1) reconnect via /v1/runs/{id}/events. Set BEFORE PrepareSSE /
	// c.JSON, both of which write headers/status to the wire.
	c.Writer.Header().Set("X-Saker-Run-Id", hubRun.ID)
	// Surface the OTel trace id so a client can stitch its own span tree
	// to the server's `runhub.publish` → `runhub.batch.flush` chain in
	// Jaeger / Tempo. Empty when no provider is installed (the global
	// noop returns an empty SpanContext); we still write the header so
	// downstream proxies see a deterministic shape and don't add their
	// own. Set BEFORE the body writes (PrepareSSE / c.JSON) — a header
	// emitted after WriteHeader is silently dropped by net/http.
	if traceID := trace.SpanContextFromContext(c.Request.Context()).TraceID(); traceID.IsValid() {
		c.Writer.Header().Set("X-Saker-Trace-Id", traceID.String())
	}

	chunkID := makeChatChunkID(hubRun.ID)

	// Inject AskQuestionFunc when tool_call mode is active. The askFn
	// publishes tool_calls chunks, pauses the run, and blocks until the
	// client submits an answer via a second POST or the submit endpoint.
	var ps *pauseSignal
	if extra.EffectiveAskUserQuestionMode() == AskQuestionToolCall {
		ps = newPauseSignal()
		askBuilder := newChatChunkBuilder(chunkID, hubRun.ID, req.Model, g.deps.Options.ErrorDetailMode)
		askFn := g.makeAskQuestionFunc(hubRun, askBuilder, ps)
		producerCtx = toolbuiltin.WithAskQuestionFunc(producerCtx, askFn)
	}

	g.deps.Logger.Info("openai gateway run starting",
		"run_id", hubRun.ID,
		"tenant", tenantID,
		"model", req.Model,
		"stream", req.Stream,
		"human_input_mode", extra.EffectiveHumanInputMode(),
		"cancel_on_disconnect", extra.EffectiveCancelOnDisconnect(),
	)

	eventCh, err := g.deps.Runtime.RunStream(producerCtx, sakerReq)
	if err != nil {
		producerCancel()
		g.hub.Finish(hubRun.ID, runhub.RunStatusFailed)
		InvalidRequest(c, err.Error())
		return
	}

	includeUsage := req.Stream && parseIncludeUsage(req.StreamOptions)

	go g.runChatProducer(eventCh, hubRun, producerCancel, chunkID, req.Model, extra.ExposeToolCalls, includeUsage)

	var pauseNotify <-chan struct{}
	if ps != nil {
		pauseNotify = ps.Ch()
	}

	if req.Stream {
		g.streamChatSSE(c, hubRun, extra, includeUsage, pauseNotify)
	} else {
		g.streamChatSync(c, hubRun, extra, req.Model, chunkID)
	}
}

// parseIncludeUsage looks up stream_options.include_usage and returns true
// only when explicitly set to true. Any non-bool / missing value yields
// false (forward-compat: unknown stream_options keys are ignored per the
// OpenAI spec).
func parseIncludeUsage(opts map[string]any) bool {
	if opts == nil {
		return false
	}
	v, ok := opts["include_usage"]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}



// streamChatSSE writes the per-run event stream to the client as SSE.
// On client disconnect, honors cancel_on_disconnect (forces true when
// human_input_mode=never, see ExtraBody.EffectiveCancelOnDisconnect).
//
// includeUsage controls whether the trailing "usage" event (always
// produced when the runtime reports any token counts) is forwarded to the
// client. OpenAI requires this frame to be opt-in via
// stream_options.include_usage; old SDKs that don't ask for it would be
// confused by an empty-choices chunk and we'd break their final-message
// detection.
func (g *Gateway) streamChatSSE(c *gin.Context, hubRun *runhub.Run, extra ExtraBody, includeUsage bool, pauseCh <-chan struct{}) {
	flusher := PrepareSSE(c)
	if flusher == nil {
		ServerError(c, "stream not supported by underlying writer")
		return
	}

	eventsCh, backfill, unsub := hubRun.Subscribe()
	defer unsub()

	emit := func(e runhub.Event) error {
		if e.Type == "usage" && !includeUsage {
			return nil
		}
		return writeChunkSSE(c.Writer, hubRun.ID, e)
	}

	// Replay any events that landed in the ring before we subscribed.
	for _, e := range backfill {
		if err := emit(e); err != nil {
			return
		}
	}
	flusher.Flush()

	keepalive := time.NewTicker(keepaliveInterval)
	defer keepalive.Stop()

	clientCtx := c.Request.Context()

	for {
		select {
		case e, ok := <-eventsCh:
			if !ok {
				// Annotate non-clean terminations (cancelled/expired/failed)
				// with an OpenAI-shaped error frame BEFORE [DONE]; lets the
				// client distinguish a killed stream from one that ran to
				// completion. See pkg/server/openai/terminal_error.go.
				_ = writeTerminalErrorIfNeeded(c.Writer, hubRun)
				_ = WriteDone(c.Writer)
				flusher.Flush()
				return
			}
			if err := emit(e); err != nil {
				return
			}
			flusher.Flush()
		case <-pauseCh:
			// Agent called ask_user_question — terminate this SSE stream.
			// The client will submit an answer and get a new stream.
			_ = WriteDone(c.Writer)
			flusher.Flush()
			return
		case <-keepalive.C:
			if err := WriteComment(c.Writer, "keepalive"); err != nil {
				return
			}
			flusher.Flush()
		case <-clientCtx.Done():
			if extra.EffectiveCancelOnDisconnect() {
				_ = g.hub.Cancel(hubRun.ID)
			}
			return
		}
	}
}

// writeChunkSSE serializes one ring event onto the SSE wire. Data is
// already JSON; we just wrap it with id: + data: lines.
//
// The id line uses the qualified format `<run_id>:<seq>`. Including the
// run id in the cursor lets parseLastEventID reject reconnect cursors
// from other runs (cross-run leak / probe protection) and gives clients
// a self-describing token they don't need to combine with separate
// state. This is a wire-protocol breaking change vs the legacy bare-int
// format; clients written against the old wire must be updated to
// extract the run id (see examples/21-openai-gateway).
func writeChunkSSE(w io.Writer, runID string, e runhub.Event) error {
	if _, err := fmt.Fprintf(w, "id: %s:%d\n", runID, e.Seq); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", e.Data); err != nil {
		return err
	}
	return nil
}

