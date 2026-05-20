package agui

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/saker-ai/saker/pkg/api"
	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

func TestHandleRun_ReconnectExpiredSession(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	_, err := RegisterAGUIGateway(engine, Deps{
		Runtime: &stubRunner{},
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Options: Options{Enabled: true, DevBypassAuth: true},
	})
	if err != nil {
		t.Fatalf("RegisterAGUIGateway: %v", err)
	}

	body := aguitypes.RunAgentInput{
		ThreadID: "thread-reconnect",
		RunID:    "run-nonexistent",
		Messages: []aguitypes.Message{
			{Role: "user", Content: "hello"},
		},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/agents/run?last_event_id=5", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Fatalf("expected 410 Gone for expired session, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err == nil {
		if resp["error"]["type"] != "session_expired" {
			t.Errorf("error type = %q, want session_expired", resp["error"]["type"])
		}
	}
}

func TestHandleRun_ReconnectSuccess(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	gw, err := RegisterAGUIGateway(engine, Deps{
		Runtime: &stubRunner{},
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Options: Options{Enabled: true, DevBypassAuth: true, DetachTimeout: 5 * time.Second},
	})
	if err != nil {
		t.Fatalf("RegisterAGUIGateway: %v", err)
	}

	// Create a session manually with buffered events.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh := make(chan api.StreamEvent)
	sideCh := make(chan sideEvent, 8)

	session := newRunSession(gw, "run-reconnect", "thread-reconnect", "turn-1", "default",
		eventCh, sideCh, ctx, cancel)
	gw.sessions.register(session)

	// Simulate some events in the ring.
	session.mu.Lock()
	session.ring.Push(1, []byte("id: 1\nevent: RUN_STARTED\ndata: {\"type\":\"RUN_STARTED\"}\n\n"))
	session.ring.Push(2, []byte("id: 2\nevent: STATE_SNAPSHOT\ndata: {\"state\":{}}\n\n"))
	session.ring.Push(3, []byte("id: 3\nevent: TEXT_MESSAGE_START\ndata: {\"type\":\"TEXT_MESSAGE_START\"}\n\n"))
	session.ring.Push(4, []byte("id: 4\nevent: TEXT_MESSAGE_CONTENT\ndata: {\"text\":\"hello\"}\n\n"))
	session.mu.Unlock()

	// Mark session as detached (simulating client disconnect).
	session.detach()

	// Reconnect with last_event_id=2.
	body := aguitypes.RunAgentInput{
		ThreadID: "thread-reconnect",
		RunID:    "run-reconnect",
		Messages: []aguitypes.Message{
			{Role: "user", Content: "hello"},
		},
	}
	b, _ := json.Marshal(body)

	reqCtx, reqCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer reqCancel()
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/run?last_event_id=2", strings.NewReader(string(b)))
	req = req.WithContext(reqCtx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for reconnect, got %d: %s", w.Code, w.Body.String())
	}

	respBody := w.Body.String()
	// Should contain replayed events 3 and 4 (after lastEventID=2).
	if !strings.Contains(respBody, "id: 3") {
		t.Error("response should contain replayed event id: 3")
	}
	if !strings.Contains(respBody, "id: 4") {
		t.Error("response should contain replayed event id: 4")
	}
	// Should NOT contain events 1 or 2 (already received by client).
	if strings.Contains(respBody, "id: 1\n") {
		t.Error("response should NOT contain event id: 1")
	}
	if strings.Contains(respBody, "id: 2\n") {
		t.Error("response should NOT contain event id: 2")
	}
	// Should have SSE content-type and retry directive.
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if !strings.Contains(respBody, "retry: 3000") {
		t.Error("response should contain retry: 3000 directive")
	}
}

func TestHandleRun_ReconnectRingOverflow(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	gw, err := RegisterAGUIGateway(engine, Deps{
		Runtime: &stubRunner{},
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Options: Options{Enabled: true, DevBypassAuth: true},
	})
	if err != nil {
		t.Fatalf("RegisterAGUIGateway: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh := make(chan api.StreamEvent)
	sideCh := make(chan sideEvent, 8)

	session := newRunSession(gw, "run-overflow", "thread-overflow", "turn-1", "default",
		eventCh, sideCh, ctx, cancel)
	gw.sessions.register(session)

	// Fill ring to overflow (default size 256).
	session.mu.Lock()
	for i := 1; i <= 260; i++ {
		session.ring.Push(i, []byte("id: X\ndata: {}\n\n"))
	}
	session.mu.Unlock()

	session.detach()

	// Try to reconnect with a very old event ID (evicted from ring).
	body := aguitypes.RunAgentInput{
		ThreadID: "thread-overflow",
		RunID:    "run-overflow",
		Messages: []aguitypes.Message{
			{Role: "user", Content: "hello"},
		},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/agents/run?last_event_id=1", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// Should get 200 with SSE error event (since headers are already sent as SSE).
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (SSE error), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ring_overflow") {
		t.Error("response should contain ring_overflow error")
	}
}

func TestCapabilities_Resumable(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	_, err := RegisterAGUIGateway(engine, Deps{
		Runtime: &stubRunner{},
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Options: Options{Enabled: true, DevBypassAuth: true},
	})
	if err != nil {
		t.Fatalf("RegisterAGUIGateway: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/agents/run/capabilities", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var caps AgentCapabilities
	if err := json.Unmarshal(w.Body.Bytes(), &caps); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if caps.Transport == nil || !caps.Transport.Resumable {
		t.Error("transport.resumable should be true")
	}
}
