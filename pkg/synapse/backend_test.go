package synapse

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	synapsev1 "github.com/saker-ai/saker/proto/synapse/v1"
)

func TestHTTPBackend_Health_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := NewHTTPBackend(srv.URL)
	if err := b.Health(context.Background()); err != nil {
		t.Fatalf("Health() = %v, want nil", err)
	}
}

func TestHTTPBackend_Health_Unhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	b := NewHTTPBackend(srv.URL)
	err := b.Health(context.Background())
	if err == nil {
		t.Fatal("Health() = nil, want error")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("error %q should mention status 503", err)
	}
}

func TestHTTPBackend_Stream_JSON(t *testing.T) {
	body := `{"choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	b := NewHTTPBackend(srv.URL)
	out := make(chan Frame, 8)
	req := Request{
		RequestID: "req-1",
		Protocol:  synapsev1.Protocol_PROTOCOL_OPENAI_CHAT,
		Path:      "/v1/chat/completions",
		Body:      []byte(`{"model":"test"}`),
	}

	err := b.Stream(context.Background(), req, out)
	if err != nil {
		t.Fatalf("Stream() = %v", err)
	}
	close(out)

	var frames []Frame
	for f := range out {
		frames = append(frames, f)
	}
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2 (chunk + done)", len(frames))
	}
	if frames[0].Chunk == nil {
		t.Fatal("first frame should be a Chunk")
	}
	if frames[0].Chunk.RequestId != "req-1" {
		t.Errorf("chunk request_id = %q, want %q", frames[0].Chunk.RequestId, "req-1")
	}
	if frames[1].Done == nil {
		t.Fatal("second frame should be Done")
	}
	if frames[1].Done.Usage == nil {
		t.Fatal("Done.Usage should not be nil")
	}
	if frames[1].Done.Usage.TotalTokens != 15 {
		t.Errorf("total_tokens = %d, want 15", frames[1].Done.Usage.TotalTokens)
	}
}

func TestHTTPBackend_Stream_SSE(t *testing.T) {
	ssePayload := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" there\"}}]}\n\ndata: [DONE]\n\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ssePayload)
	}))
	defer srv.Close()

	b := NewHTTPBackend(srv.URL)
	out := make(chan Frame, 16)
	req := Request{
		RequestID: "req-sse",
		Protocol:  synapsev1.Protocol_PROTOCOL_OPENAI_CHAT,
		Path:      "/v1/chat/completions",
		Body:      []byte(`{"model":"test","stream":true}`),
	}

	err := b.Stream(context.Background(), req, out)
	if err != nil {
		t.Fatalf("Stream() = %v", err)
	}
	close(out)

	var chunks, dones int
	for f := range out {
		if f.Chunk != nil {
			chunks++
		}
		if f.Done != nil {
			dones++
		}
	}
	if chunks != 2 {
		t.Errorf("got %d chunks, want 2", chunks)
	}
	if dones != 1 {
		t.Errorf("got %d dones, want 1", dones)
	}
}

func TestHTTPBackend_Stream_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
	}))
	defer srv.Close()

	b := NewHTTPBackend(srv.URL)
	out := make(chan Frame, 8)
	req := Request{RequestID: "req-err", Path: "/v1/chat/completions"}

	err := b.Stream(context.Background(), req, out)
	if err != nil {
		t.Fatalf("Stream() should return nil (error in frame), got %v", err)
	}
	close(out)

	var frames []Frame
	for f := range out {
		frames = append(frames, f)
	}
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want 1 error frame", len(frames))
	}
	if frames[0].Error == nil {
		t.Fatal("expected error frame")
	}
	if frames[0].Error.HttpStatus != 500 {
		t.Errorf("http_status = %d, want 500", frames[0].Error.HttpStatus)
	}
}

func TestHTTPBackend_Stream_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	b := NewHTTPBackend(srv.URL)
	out := make(chan Frame, 8)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := b.Stream(ctx, Request{RequestID: "req-cancel", Path: "/v1/chat/completions"}, out)
	if err == nil {
		t.Fatal("Stream() with cancelled ctx should return error")
	}
}
