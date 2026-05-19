package synapse

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	synapsev1 "github.com/saker-ai/saker/proto/synapse/v1"
	"google.golang.org/grpc/metadata"
)

// mockStream implements synapsev1.SynapseHub_RegisterClient for testing.
type mockStream struct {
	recvCh chan *synapsev1.HubMessage
	sendCh chan *synapsev1.SakerMessage
	ctx    context.Context
	mu     sync.Mutex
	closed bool
}

func newMockStream(ctx context.Context) *mockStream {
	return &mockStream{
		recvCh: make(chan *synapsev1.HubMessage, 32),
		sendCh: make(chan *synapsev1.SakerMessage, 64),
		ctx:    ctx,
	}
}

func (m *mockStream) Send(msg *synapsev1.SakerMessage) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return io.EOF
	}
	m.mu.Unlock()
	select {
	case m.sendCh <- msg:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

func (m *mockStream) Recv() (*synapsev1.HubMessage, error) {
	select {
	case msg, ok := <-m.recvCh:
		if !ok {
			return nil, io.EOF
		}
		return msg, nil
	case <-m.ctx.Done():
		return nil, m.ctx.Err()
	}
}

func (m *mockStream) Header() (metadata.MD, error) { return nil, nil }
func (m *mockStream) Trailer() metadata.MD          { return nil }
func (m *mockStream) CloseSend() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}
func (m *mockStream) Context() context.Context  { return m.ctx }
func (m *mockStream) SendMsg(_ any) error       { return nil }
func (m *mockStream) RecvMsg(_ any) error       { return nil }

// mockBackend implements Backend for testing.
type mockBackend struct {
	streamFunc func(ctx context.Context, req Request, out chan<- Frame) error
	healthFunc func(ctx context.Context) error
}

func (b *mockBackend) Stream(ctx context.Context, req Request, out chan<- Frame) error {
	if b.streamFunc != nil {
		return b.streamFunc(ctx, req, out)
	}
	return nil
}

func (b *mockBackend) Health(ctx context.Context) error {
	if b.healthFunc != nil {
		return b.healthFunc(ctx)
	}
	return nil
}

func TestPump_HappyPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := newMockStream(ctx)
	backend := &mockBackend{
		streamFunc: func(_ context.Context, req Request, out chan<- Frame) error {
			out <- Frame{Chunk: &synapsev1.ChatChunk{
				RequestId: req.RequestID, Data: []byte("response data"),
			}}
			out <- Frame{Done: &synapsev1.ChatDone{RequestId: req.RequestID}}
			return nil
		},
	}

	pump := NewPump(PumpOptions{
		Stream:    stream,
		Backend:   backend,
		Heartbeat: time.Hour,
	})

	go func() {
		stream.recvCh <- &synapsev1.HubMessage{
			Payload: &synapsev1.HubMessage_Request{Request: &synapsev1.ChatRequest{
				RequestId: "test-req-1",
				Payload:   []byte(`{"model":"test"}`),
			}},
		}
		time.Sleep(100 * time.Millisecond)
		close(stream.recvCh)
	}()

	err := pump.Run(ctx)
	if err != nil {
		t.Fatalf("Pump.Run() = %v, want nil", err)
	}

	var chunks, dones int
	close(stream.sendCh)
	for msg := range stream.sendCh {
		switch msg.Payload.(type) {
		case *synapsev1.SakerMessage_Chunk:
			chunks++
		case *synapsev1.SakerMessage_Done:
			dones++
		}
	}
	if chunks != 1 {
		t.Errorf("sent %d chunks, want 1", chunks)
	}
	if dones != 1 {
		t.Errorf("sent %d dones, want 1", dones)
	}
}

func TestPump_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockStream(ctx)
	backend := &mockBackend{}

	pump := NewPump(PumpOptions{
		Stream:    stream,
		Backend:   backend,
		Heartbeat: time.Hour,
	})

	done := make(chan error, 1)
	go func() { done <- pump.Run(ctx) }()

	cancel()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Errorf("Run() = %v, want nil or context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pump did not exit after context cancel")
	}
}

func TestPump_Heartbeat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream := newMockStream(ctx)
	backend := &mockBackend{}

	pump := NewPump(PumpOptions{
		Stream:    stream,
		Backend:   backend,
		Heartbeat: 50 * time.Millisecond,
	})

	go func() {
		time.Sleep(200 * time.Millisecond)
		close(stream.recvCh)
	}()

	_ = pump.Run(ctx)

	close(stream.sendCh)
	var heartbeats int
	for msg := range stream.sendCh {
		if _, ok := msg.Payload.(*synapsev1.SakerMessage_Heartbeat); ok {
			heartbeats++
		}
	}
	if heartbeats < 2 {
		t.Errorf("got %d heartbeats in 200ms with 50ms interval, want >= 2", heartbeats)
	}
}

func TestPump_BackendError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := newMockStream(ctx)
	backend := &mockBackend{
		streamFunc: func(_ context.Context, req Request, out chan<- Frame) error {
			out <- Frame{Error: &synapsev1.ChatError{
				RequestId: req.RequestID, Code: "test_error",
				Message: "something failed", HttpStatus: 500,
			}}
			return nil
		},
	}

	pump := NewPump(PumpOptions{
		Stream:    stream,
		Backend:   backend,
		Heartbeat: time.Hour,
	})

	go func() {
		stream.recvCh <- &synapsev1.HubMessage{
			Payload: &synapsev1.HubMessage_Request{Request: &synapsev1.ChatRequest{
				RequestId: "err-req",
				Payload:   []byte(`{}`),
			}},
		}
		time.Sleep(100 * time.Millisecond)
		close(stream.recvCh)
	}()

	_ = pump.Run(ctx)

	close(stream.sendCh)
	var errFrames int
	for msg := range stream.sendCh {
		if e, ok := msg.Payload.(*synapsev1.SakerMessage_Error); ok {
			errFrames++
			if e.Error.Code != "test_error" {
				t.Errorf("error code = %q, want %q", e.Error.Code, "test_error")
			}
		}
	}
	if errFrames != 1 {
		t.Errorf("got %d error frames, want 1", errFrames)
	}
}

func TestPump_PingPong(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := newMockStream(ctx)
	backend := &mockBackend{}

	pump := NewPump(PumpOptions{
		Stream:    stream,
		Backend:   backend,
		Heartbeat: time.Hour,
	})

	go func() {
		stream.recvCh <- &synapsev1.HubMessage{
			Payload: &synapsev1.HubMessage_Ping{Ping: &synapsev1.Ping{UnixNanos: 12345}},
		}
		time.Sleep(50 * time.Millisecond)
		close(stream.recvCh)
	}()

	_ = pump.Run(ctx)

	close(stream.sendCh)
	var pongs int
	for msg := range stream.sendCh {
		if _, ok := msg.Payload.(*synapsev1.SakerMessage_Pong); ok {
			pongs++
		}
	}
	if pongs != 1 {
		t.Errorf("got %d pongs, want 1", pongs)
	}
}

func TestPump_Cancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := newMockStream(ctx)

	started := make(chan struct{})
	backend := &mockBackend{
		streamFunc: func(ctx context.Context, req Request, out chan<- Frame) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	}

	pump := NewPump(PumpOptions{
		Stream:    stream,
		Backend:   backend,
		Heartbeat: time.Hour,
	})

	go func() {
		stream.recvCh <- &synapsev1.HubMessage{
			Payload: &synapsev1.HubMessage_Request{Request: &synapsev1.ChatRequest{
				RequestId: "cancel-req",
				Payload:   []byte(`{}`),
			}},
		}
		<-started
		stream.recvCh <- &synapsev1.HubMessage{
			Payload: &synapsev1.HubMessage_Cancel{Cancel: &synapsev1.CancelRequest{
				RequestId: "cancel-req",
			}},
		}
		time.Sleep(100 * time.Millisecond)
		close(stream.recvCh)
	}()

	_ = pump.Run(ctx)

	close(stream.sendCh)
	var cancelled bool
	for msg := range stream.sendCh {
		if e, ok := msg.Payload.(*synapsev1.SakerMessage_Error); ok {
			if e.Error.Code == "cancelled" {
				cancelled = true
			}
		}
	}
	if !cancelled {
		t.Error("expected a cancelled error frame after CancelRequest")
	}
}
