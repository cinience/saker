package agui

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/saker-ai/saker/pkg/api"
)

type blockingRunner struct {
	started chan struct{}
	once    sync.Once
}

func (r *blockingRunner) RunStream(ctx context.Context, _ api.Request) (<-chan api.StreamEvent, error) {
	r.once.Do(func() { close(r.started) })
	ch := make(chan api.StreamEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func TestGatewayShutdownCancelsActiveRun(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runner := &blockingRunner{started: make(chan struct{})}
	engine := gin.New()
	gw, err := RegisterAGUIGateway(engine, Deps{
		Runtime: runner,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Options: Options{Enabled: true, DevBypassAuth: true},
	})
	if err != nil {
		t.Fatalf("RegisterAGUIGateway: %v", err)
	}

	body := []byte(`{"threadId":"thread_shutdown","runId":"run_shutdown","messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/run", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		engine.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("run did not start")
	}

	gw.Shutdown()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("active AG-UI run did not exit after gateway shutdown")
	}
}
