package agui

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/saker-ai/saker/pkg/api"
)

func newTestGateway() *Gateway {
	return &Gateway{
		deps: Deps{
			Runtime: &stubRunner{},
			Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
			Options: Options{Enabled: true, DetachTimeout: 500 * time.Millisecond},
		},
		activeCancels: make(map[string]context.CancelFunc),
		threadRuns:    make(map[string]string),
		sessions:      newRunSessionManager(),
		artifactCache: newArtifactCache(),
	}
}

func TestRunSession_DetachAndReattach(t *testing.T) {
	gw := newTestGateway()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh := make(chan api.StreamEvent)
	sideCh := make(chan sideEvent, 8)

	session := newRunSession(gw, "run_1", "thread_1", "turn_1", "default",
		eventCh, sideCh, ctx, cancel)
	gw.sessions.register(session)

	// Simulate pump writing some events to the ring.
	session.mu.Lock()
	session.ring.Push(1, []byte("id: 1\nevent: RUN_STARTED\ndata: {}\n\n"))
	session.ring.Push(2, []byte("id: 2\nevent: STATE_SNAPSHOT\ndata: {}\n\n"))
	session.ring.Push(3, []byte("id: 3\nevent: TEXT_MESSAGE_START\ndata: {}\n\n"))
	session.mu.Unlock()

	// Simulate client disconnect.
	session.detach()
	if session.getState() != sessionDetached {
		t.Fatal("expected sessionDetached state after detach")
	}

	// Reconnect with lastEventID=1 (should replay events 2 and 3).
	var buf bytes.Buffer
	client := &attachedClient{
		writer:  &buf,
		flusher: &noopFlusher{},
		doneCh:  make(chan struct{}),
	}

	if err := session.replayAndAttach(client, 1); err != nil {
		t.Fatalf("replayAndAttach: %v", err)
	}

	if session.getState() != sessionRunning {
		t.Errorf("expected sessionRunning after reattach, got %d", session.getState())
	}

	replayed := buf.String()
	if !bytes.Contains([]byte(replayed), []byte("id: 2")) {
		t.Error("replay should contain event id: 2")
	}
	if !bytes.Contains([]byte(replayed), []byte("id: 3")) {
		t.Error("replay should contain event id: 3")
	}
	if bytes.Contains([]byte(replayed), []byte("id: 1")) {
		t.Error("replay should NOT contain event id: 1 (already received)")
	}
}

func TestRunSession_DetachTimeoutCancelsRuntime(t *testing.T) {
	gw := newTestGateway()
	gw.deps.Options.DetachTimeout = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh := make(chan api.StreamEvent)
	sideCh := make(chan sideEvent, 8)

	session := newRunSession(gw, "run_2", "thread_2", "turn_2", "default",
		eventCh, sideCh, ctx, cancel)
	gw.sessions.register(session)

	// Attach then detach (starts detach timer).
	client := &attachedClient{
		writer:  io.Discard,
		flusher: &noopFlusher{},
		doneCh:  make(chan struct{}),
	}
	session.attach(client)
	session.detach()

	// Wait for detach timeout.
	time.Sleep(200 * time.Millisecond)

	if session.getState() != sessionExpired {
		t.Errorf("expected sessionExpired, got %d", session.getState())
	}

	// Runtime context should be cancelled.
	if ctx.Err() == nil {
		t.Error("runtime context should be cancelled after detach timeout")
	}
}

func TestRunSession_RingOverflowReturnsError(t *testing.T) {
	gw := newTestGateway()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh := make(chan api.StreamEvent)
	sideCh := make(chan sideEvent, 8)

	session := newRunSession(gw, "run_3", "thread_3", "turn_3", "default",
		eventCh, sideCh, ctx, cancel)

	// Fill ring so oldest seq is high.
	session.mu.Lock()
	for i := 1; i <= 260; i++ {
		session.ring.Push(i, []byte("data\n\n"))
	}
	session.mu.Unlock()

	var buf bytes.Buffer
	client := &attachedClient{
		writer:  &buf,
		flusher: &noopFlusher{},
		doneCh:  make(chan struct{}),
	}

	// Try to reconnect with a seq that's been evicted.
	err := session.replayAndAttach(client, 1)
	if err == nil {
		t.Fatal("expected ring overflow error, got nil")
	}
}

func TestRunSession_PumpBuffersWithoutClient(t *testing.T) {
	gw := newTestGateway()

	baseCtx, baseCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer baseCancel()
	runtimeCtx, runtimeCancel := context.WithCancel(baseCtx)

	eventCh := make(chan api.StreamEvent, 10)
	sideCh := make(chan sideEvent, 8)

	session := newRunSession(gw, "run_4", "thread_4", "turn_4", "default",
		eventCh, sideCh, runtimeCtx, runtimeCancel)
	gw.sessions.register(session)

	// Register as active run.
	gw.mu.Lock()
	gw.activeCancels["run_4"] = runtimeCancel
	gw.activeWg.Add(1)
	gw.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		session.pump(func() {
			runtimeCancel()
			gw.mu.Lock()
			delete(gw.activeCancels, "run_4")
			gw.mu.Unlock()
			gw.activeWg.Done()
		})
	}()

	// Send some events without a client attached.
	eventCh <- api.StreamEvent{Type: api.EventContentBlockDelta, Delta: &api.Delta{Text: "hello"}}
	eventCh <- api.StreamEvent{Type: api.EventContentBlockDelta, Delta: &api.Delta{Text: " world"}}

	// Give pump time to process.
	time.Sleep(50 * time.Millisecond)

	// Close the runtime stream to trigger completion.
	close(eventCh)

	// Wait for pump to finish.
	wg.Wait()

	// Ring should have events buffered (initial events + text events + finalize).
	session.mu.Lock()
	ringLen := session.ring.Len()
	session.mu.Unlock()

	if ringLen < 3 {
		t.Errorf("expected at least 3 events in ring, got %d", ringLen)
	}
}

func TestEventRing_OldestSeq(t *testing.T) {
	r := newEventRing(4)

	if r.OldestSeq() != 0 {
		t.Errorf("empty ring OldestSeq = %d, want 0", r.OldestSeq())
	}

	r.Push(1, nil)
	r.Push(2, nil)
	if r.OldestSeq() != 1 {
		t.Errorf("OldestSeq = %d, want 1", r.OldestSeq())
	}

	// Fill and wrap.
	r.Push(3, nil)
	r.Push(4, nil)
	r.Push(5, nil) // Overwrites slot 0 (which had seq 1).
	if r.OldestSeq() != 2 {
		t.Errorf("after wrap OldestSeq = %d, want 2", r.OldestSeq())
	}
}

func TestEventRing_NewestSeq(t *testing.T) {
	r := newEventRing(4)

	if r.NewestSeq() != 0 {
		t.Errorf("empty ring NewestSeq = %d, want 0", r.NewestSeq())
	}

	r.Push(1, nil)
	r.Push(2, nil)
	r.Push(3, nil)
	if r.NewestSeq() != 3 {
		t.Errorf("NewestSeq = %d, want 3", r.NewestSeq())
	}

	// Wrap around.
	r.Push(4, nil)
	r.Push(5, nil)
	if r.NewestSeq() != 5 {
		t.Errorf("after wrap NewestSeq = %d, want 5", r.NewestSeq())
	}
}

type noopFlusher struct{}

func (f *noopFlusher) Flush()                         {}
func (f *noopFlusher) Header() http.Header            { return http.Header{} }
func (f *noopFlusher) Write(b []byte) (int, error)    { return len(b), nil }
func (f *noopFlusher) WriteHeader(statusCode int)     {}
