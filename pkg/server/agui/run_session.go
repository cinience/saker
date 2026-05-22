package agui

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/saker-ai/saker/pkg/api"
)

const (
	defaultDetachTimeout = 30 * time.Second
	sessionGracePeriod   = 30 * time.Second
)

type sessionState int

const (
	sessionRunning  sessionState = iota // Runtime is executing, client may or may not be attached
	sessionDetached                     // Client disconnected, detach timer running
	sessionFinished                     // Runtime completed (success or error)
	sessionExpired                      // Detach timer fired, runtime cancelled
)

// attachedClient represents a connected SSE client.
type attachedClient struct {
	writer  io.Writer
	flusher http.Flusher
	doneCh  chan struct{} // closed when client disconnects
}

// runSession owns the runtime lifecycle independently of any HTTP connection.
// Events are always buffered in the ring; when a client is attached, events
// are also forwarded to the wire.
type runSession struct {
	mu sync.Mutex

	// Identity
	runID     string
	threadID  string
	turnID    string
	projectID string

	// Lifecycle
	state         sessionState
	runtimeCtx    context.Context
	runtimeCancel context.CancelFunc
	detachTimer   *time.Timer
	detachTimeout time.Duration

	// Event buffering
	ring     *eventRing
	eventSeq int

	// Client output (nil when detached)
	client *attachedClient

	// Runtime channels
	eventCh <-chan api.StreamEvent
	sideCh  chan sideEvent

	// Pump signals
	clientNotify chan struct{} // poked when client attaches/detaches
	doneCh       chan struct{} // closed when pump exits

	// Dependencies
	gateway *Gateway
	logger  *slog.Logger

	// mcpRegistry holds per-session MCP connections from client ForwardedProps.
	mcpRegistry *SessionMCPRegistry
}

func newRunSession(g *Gateway, runID, threadID, turnID, projectID string, eventCh <-chan api.StreamEvent, sideCh chan sideEvent, runtimeCtx context.Context, runtimeCancel context.CancelFunc) *runSession {
	detachTimeout := g.deps.Options.DetachTimeout
	if detachTimeout == 0 {
		detachTimeout = defaultDetachTimeout
	}
	return &runSession{
		runID:         runID,
		threadID:      threadID,
		turnID:        turnID,
		projectID:     projectID,
		state:         sessionRunning,
		runtimeCtx:    runtimeCtx,
		runtimeCancel: runtimeCancel,
		detachTimeout: detachTimeout,
		ring:          newEventRing(defaultEventRingSize),
		eventCh:       eventCh,
		sideCh:        sideCh,
		clientNotify:  make(chan struct{}, 1),
		doneCh:        make(chan struct{}),
		gateway:       g,
		logger:        g.deps.Logger,
	}
}

// attach sets the client writer for live event forwarding.
func (s *runSession) attach(client *attachedClient) {
	s.mu.Lock()
	s.client = client
	if s.detachTimer != nil {
		s.detachTimer.Stop()
		s.detachTimer = nil
	}
	if s.state == sessionDetached {
		s.state = sessionRunning
	}
	s.mu.Unlock()
	// Notify pump that a client is now attached.
	select {
	case s.clientNotify <- struct{}{}:
	default:
	}
}

// detach removes the client writer and starts the detach timer.
func (s *runSession) detach() {
	s.mu.Lock()
	s.client = nil
	if s.state == sessionRunning {
		s.state = sessionDetached
		aguiSessionDetachedTotal.Inc()
		s.detachTimer = time.AfterFunc(s.detachTimeout, s.onDetachTimeout)
	}
	s.mu.Unlock()
}

func (s *runSession) onDetachTimeout() {
	s.mu.Lock()
	if s.state != sessionDetached {
		s.mu.Unlock()
		return
	}
	s.state = sessionExpired
	aguiSessionExpiredTotal.Inc()
	s.mu.Unlock()
	s.logger.Info("agui session detach timeout expired, cancelling runtime",
		"run_id", s.runID, "thread_id", s.threadID)
	s.runtimeCancel()
}

// replayAndAttach replays buffered events since lastEventID to the new client,
// then attaches for live forwarding.
// Returns error if ring has overflowed past lastEventID.
func (s *runSession) replayAndAttach(client *attachedClient, lastEventID int) error {
	s.mu.Lock()
	oldest := s.ring.OldestSeq()
	if oldest > 0 && lastEventID < oldest {
		s.mu.Unlock()
		aguiReconnectOverflowTotal.Inc()
		return fmt.Errorf("ring overflow: requested seq %d but oldest is %d", lastEventID, oldest)
	}
	entries := s.ring.Since(lastEventID)
	s.mu.Unlock()

	// Replay buffered frames (already formatted as complete SSE frames).
	for _, e := range entries {
		if _, err := client.writer.Write(e.data); err != nil {
			return fmt.Errorf("replay write failed: %w", err)
		}
	}
	if client.flusher != nil {
		client.flusher.Flush()
	}

	s.attach(client)
	aguiReconnectSuccessTotal.Inc()
	return nil
}

// getClient returns the current attached client (or nil).
func (s *runSession) getClient() *attachedClient {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client
}

// getState returns the current session state.
func (s *runSession) getState() sessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// pushToRing stores a complete SSE frame in the ring buffer.
func (s *runSession) pushToRing(seq int, frame []byte) {
	s.mu.Lock()
	s.ring.Push(seq, frame)
	s.mu.Unlock()
}

// isFinishedOrExpired returns true if the session is done.
func (s *runSession) isFinishedOrExpired() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == sessionFinished || s.state == sessionExpired
}

// markFinished sets session state to finished.
func (s *runSession) markFinished() {
	s.mu.Lock()
	if s.detachTimer != nil {
		s.detachTimer.Stop()
		s.detachTimer = nil
	}
	s.state = sessionFinished
	s.mu.Unlock()
}

// runSessionManager manages active run sessions.
type runSessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*runSession // keyed by runID
}

func newRunSessionManager() *runSessionManager {
	return &runSessionManager{sessions: make(map[string]*runSession)}
}

func (m *runSessionManager) get(runID string) *runSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[runID]
}

func (m *runSessionManager) register(s *runSession) {
	m.mu.Lock()
	m.sessions[s.runID] = s
	m.mu.Unlock()
}

func (m *runSessionManager) remove(runID string) {
	m.mu.Lock()
	delete(m.sessions, runID)
	m.mu.Unlock()
}

func (m *runSessionManager) count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// cancelAll cancels all active session runtimes (used during shutdown).
func (m *runSessionManager) cancelAll() {
	m.mu.Lock()
	sessions := make([]*runSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()
	for _, s := range sessions {
		s.runtimeCancel()
	}
}
