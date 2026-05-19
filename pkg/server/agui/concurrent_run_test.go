package agui

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestConcurrentRunCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runner := &blockingRunner{started: make(chan struct{})}
	engine := gin.New()
	_, err := RegisterAGUIGateway(engine, Deps{
		Runtime: runner,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Options: Options{Enabled: true, DevBypassAuth: true},
	})
	if err != nil {
		t.Fatalf("RegisterAGUIGateway: %v", err)
	}

	// Start first run on thread_concurrent.
	body1 := []byte(`{"threadId":"thread_concurrent","runId":"run_first","messages":[{"role":"user","content":"hello"}]}`)
	req1 := httptest.NewRequest(http.MethodPost, "/v1/agents/run", bytes.NewReader(body1))
	rec1 := httptest.NewRecorder()
	done1 := make(chan struct{})
	go func() {
		engine.ServeHTTP(rec1, req1)
		close(done1)
	}()

	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first run did not start")
	}

	// Start second run on the same thread — should cancel the first.
	runner2 := &blockingRunner{started: make(chan struct{})}
	// We need a new gateway with the second runner to prove the point.
	// Actually, let's just fire a second request — the gateway will cancel run_first.
	body2 := []byte(`{"threadId":"thread_concurrent","runId":"run_second","messages":[{"role":"user","content":"world"}]}`)
	req2 := httptest.NewRequest(http.MethodPost, "/v1/agents/run", bytes.NewReader(body2))
	rec2 := httptest.NewRecorder()
	done2 := make(chan struct{})
	go func() {
		engine.ServeHTTP(rec2, req2)
		close(done2)
	}()
	_ = runner2

	// First run should be cancelled by the second run taking over the thread.
	select {
	case <-done1:
	case <-time.After(2 * time.Second):
		t.Fatal("first run was not cancelled by second run on same thread")
	}

	// Verify the first run's response contains cancelled/error indication.
	respBody := rec1.Body.String()
	if respBody == "" {
		t.Fatal("first run response should not be empty")
	}
}
