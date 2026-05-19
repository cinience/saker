package agui

import (
	"context"
	"encoding/json"
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

type stubRunner struct{}

func (s *stubRunner) RunStream(ctx context.Context, req api.Request) (<-chan api.StreamEvent, error) {
	ch := make(chan api.StreamEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func setupGatewayForTest(t *testing.T, opts Options) (*Gateway, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	opts.Enabled = true
	deps := Deps{
		Runtime: &stubRunner{},
		Logger:  slog.Default(),
		Options: opts,
	}
	gw, err := RegisterAGUIGateway(engine, deps)
	if err != nil {
		t.Fatalf("RegisterAGUIGateway: %v", err)
	}
	return gw, engine
}

func TestLoadShedding_RejectsAtCapacity(t *testing.T) {
	t.Parallel()
	gw, engine := setupGatewayForTest(t, Options{
		MaxActiveStreams: 1,
		DevBypassAuth:   true,
	})

	// Simulate an active run occupying a slot.
	gw.mu.Lock()
	gw.activeCancels["existing_run"] = func() {}
	gw.mu.Unlock()

	body := aguitypes.RunAgentInput{
		ThreadID: "thread-test",
		RunID:    "run-test",
		Messages: []aguitypes.Message{
			{Role: "user", Content: "hello"},
		},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/agents/run", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") != "5" {
		t.Errorf("Retry-After = %q, want \"5\"", w.Header().Get("Retry-After"))
	}

	var resp map[string]map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err == nil {
		if resp["error"]["type"] != "capacity_error" {
			t.Errorf("error type = %q, want capacity_error", resp["error"]["type"])
		}
	}

	// Cleanup.
	gw.mu.Lock()
	delete(gw.activeCancels, "existing_run")
	gw.mu.Unlock()
}

func TestLoadShedding_AllowsUnderCapacity(t *testing.T) {
	t.Parallel()
	_, engine := setupGatewayForTest(t, Options{
		MaxActiveStreams: 10,
		DevBypassAuth:   true,
	})

	body := aguitypes.RunAgentInput{
		ThreadID: "thread-ok",
		RunID:    "run-ok",
		Messages: []aguitypes.Message{
			{Role: "user", Content: "hello"},
		},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/agents/run", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Use a context with a short timeout so the stream doesn't hang.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	engine.ServeHTTP(w, req)

	// Should NOT be 503 — it should start the SSE stream (200).
	if w.Code == http.StatusServiceUnavailable {
		t.Fatal("should not reject when under capacity")
	}
}

func TestEffectiveTurnTimeout_Default(t *testing.T) {
	t.Parallel()
	gw := &Gateway{deps: Deps{Options: Options{}}}
	input := aguitypes.RunAgentInput{}
	got := gw.effectiveTurnTimeout(input)
	if got == 0 {
		t.Fatal("should return non-zero default timeout")
	}
}

func TestEffectiveTurnTimeout_OperatorCap(t *testing.T) {
	t.Parallel()
	gw := &Gateway{deps: Deps{Options: Options{TurnTimeout: 5 * time.Minute}}}
	input := aguitypes.RunAgentInput{}
	got := gw.effectiveTurnTimeout(input)
	if got != 5*time.Minute {
		t.Fatalf("got %v, want 5m", got)
	}
}

func TestEffectiveTurnTimeout_ClientShorter(t *testing.T) {
	t.Parallel()
	gw := &Gateway{deps: Deps{Options: Options{TurnTimeout: 10 * time.Minute}}}
	input := aguitypes.RunAgentInput{
		ForwardedProps: map[string]any{"timeout_seconds": float64(120)},
	}
	got := gw.effectiveTurnTimeout(input)
	if got != 2*time.Minute {
		t.Fatalf("got %v, want 2m", got)
	}
}

func TestEffectiveTurnTimeout_ClientLongerCapped(t *testing.T) {
	t.Parallel()
	gw := &Gateway{deps: Deps{Options: Options{TurnTimeout: 5 * time.Minute}}}
	input := aguitypes.RunAgentInput{
		ForwardedProps: map[string]any{"timeout_seconds": float64(600)},
	}
	got := gw.effectiveTurnTimeout(input)
	if got != 5*time.Minute {
		t.Fatalf("got %v, want 5m (capped)", got)
	}
}
