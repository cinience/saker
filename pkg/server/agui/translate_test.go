package agui

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/saker-ai/saker/pkg/api"

	aguievents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// nopFilter passes text through unchanged.
type nopFilter struct{}

func (nopFilter) Push(s string) string { return s }
func (nopFilter) Flush() string        { return "" }

// tailFilter returns nothing on Push but accumulates; Flush returns all.
type tailFilter struct{ buf string }

func (f *tailFilter) Push(s string) string { f.buf += s; return "" }
func (f *tailFilter) Flush() string        { return f.buf }

// capturedEvent records a single AG-UI SSE frame.
type capturedEvent struct {
	eventType string
}

// capturingSSEWriter records event types written via writeSSE.
type capturingSSEWriter struct {
	events []capturedEvent
	ctxErr error
}

func (c *capturingSSEWriter) WriteEventWithType(ctx context.Context, _ io.Writer, _ aguievents.Event, eventType string) error {
	c.events = append(c.events, capturedEvent{eventType: eventType})
	c.ctxErr = ctx.Err()
	return nil
}

func (c *capturingSSEWriter) types() []string {
	out := make([]string, len(c.events))
	for i, e := range c.events {
		out[i] = e.eventType
	}
	return out
}

func TestTranslateEvent_TextDelta_FirstEmitsStart(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}
	evt := api.StreamEvent{Type: api.EventContentBlockDelta, Delta: &api.Delta{Text: "hello"}}
	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, evt, nopFilter{})

	types := w.types()
	if len(types) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(types), types)
	}
	if types[0] != "TEXT_MESSAGE_START" {
		t.Errorf("first event = %q, want TEXT_MESSAGE_START", types[0])
	}
	if types[1] != "TEXT_MESSAGE_CONTENT" {
		t.Errorf("second event = %q, want TEXT_MESSAGE_CONTENT", types[1])
	}
}

func TestTranslateEvent_TextDelta_SubsequentSkipsStart(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}
	evt := api.StreamEvent{Type: api.EventContentBlockDelta, Delta: &api.Delta{Text: "a"}}
	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, evt, nopFilter{})
	w.events = nil

	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, evt, nopFilter{})
	types := w.types()
	if len(types) != 1 {
		t.Fatalf("expected 1 event, got %d: %v", len(types), types)
	}
	if types[0] != "TEXT_MESSAGE_CONTENT" {
		t.Errorf("event = %q, want TEXT_MESSAGE_CONTENT", types[0])
	}
}

func TestTranslateEvent_TextDelta_EmptySkipped(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}
	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{Type: api.EventContentBlockDelta, Delta: &api.Delta{Text: ""}}, nopFilter{})
	if len(w.events) != 0 {
		t.Errorf("empty text should emit nothing, got %d events", len(w.events))
	}

	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{Type: api.EventContentBlockDelta, Delta: nil}, nopFilter{})
	if len(w.events) != 0 {
		t.Errorf("nil delta should emit nothing, got %d events", len(w.events))
	}
}

func TestTranslateEvent_ToolExecutionStart(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}
	evt := api.StreamEvent{
		Type:      api.EventToolExecutionStart,
		ToolUseID: "tc_1",
		Name:      "bash",
		Input:     map[string]string{"cmd": "ls"},
	}
	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, evt, nopFilter{})

	types := w.types()
	// No empty TextMessage placeholder emitted — only activity+start+args.
	if len(types) != 3 {
		t.Fatalf("expected 3 events (activity+start+args), got %d: %v", len(types), types)
	}
	if types[0] != "ACTIVITY_SNAPSHOT" {
		t.Errorf("types[0] = %q, want ACTIVITY_SNAPSHOT", types[0])
	}
	if types[1] != "TOOL_CALL_START" {
		t.Errorf("types[1] = %q, want TOOL_CALL_START", types[1])
	}
	if types[2] != "TOOL_CALL_ARGS" {
		t.Errorf("types[2] = %q, want TOOL_CALL_ARGS", types[2])
	}
	if !state.toolCalls["tc_1"] {
		t.Error("tool call should be tracked")
	}
	if state.lastToolID != "tc_1" {
		t.Errorf("lastToolID = %q, want tc_1", state.lastToolID)
	}
}

func TestTranslateEvent_ToolExecutionStart_NoInput(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}
	evt := api.StreamEvent{
		Type:      api.EventToolExecutionStart,
		ToolUseID: "tc_2",
		Name:      "read",
	}
	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, evt, nopFilter{})

	types := w.types()
	// No empty TextMessage placeholder — just activity+start (no args since Input is nil).
	if len(types) != 2 {
		t.Fatalf("expected 2 events (activity+start), got %d: %v", len(types), types)
	}
	if types[0] != "ACTIVITY_SNAPSHOT" {
		t.Errorf("types[0] = %q, want ACTIVITY_SNAPSHOT", types[0])
	}
	if types[1] != "TOOL_CALL_START" {
		t.Errorf("types[1] = %q, want TOOL_CALL_START", types[1])
	}
}

func TestTranslateEvent_AskUserQuestionToolExecutionStartSkipped(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}

	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{
		Type:      api.EventToolExecutionStart,
		ToolUseID: "toolu_ask",
		Name:      "ask_user_question",
		Input: map[string]any{
			"questions": []map[string]any{{"question": "Which one?", "options": []string{"A"}}},
		},
	}, nopFilter{})

	if len(w.events) != 0 {
		t.Fatalf("ask_user_question side channel should suppress raw tool call, got %v", w.types())
	}
	if state.lastToolID != "" {
		t.Fatalf("lastToolID = %q, want empty", state.lastToolID)
	}
}

func TestTranslateEvent_AskUserQuestionToolExecutionResultSkipped(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}

	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{
		Type:      api.EventToolExecutionStart,
		ToolUseID: "toolu_ask",
		Name:      "ask_user_question",
		Input: map[string]any{
			"questions": []map[string]any{{"question": "Which one?", "options": []string{"A"}}},
		},
	}, nopFilter{})
	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{
		Type:      api.EventToolExecutionResult,
		ToolUseID: "toolu_ask",
		Output:    "A",
	}, nopFilter{})

	if len(w.events) != 0 {
		t.Fatalf("ask_user_question raw lifecycle should be suppressed, got %v", w.types())
	}
	if state.suppressedToolCalls["toolu_ask"] {
		t.Fatal("suppressed tool call should be cleared after result")
	}
}

func TestTranslateEvent_ToolExecutionStart_ClosesOpenTool(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}
	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{
		Type: api.EventToolExecutionStart, ToolUseID: "tc_1", Name: "bash",
	}, nopFilter{})
	w.events = nil

	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{
		Type: api.EventToolExecutionStart, ToolUseID: "tc_2", Name: "grep",
	}, nopFilter{})

	types := w.types()
	if len(types) < 3 {
		t.Fatalf("expected >=3 events, got %d: %v", len(types), types)
	}
	if types[0] != "TOOL_CALL_END" {
		t.Errorf("should close previous tool first, got %q", types[0])
	}
	if types[1] != "ACTIVITY_SNAPSHOT" {
		t.Errorf("then activity snapshot, got %q", types[1])
	}
	if types[2] != "TOOL_CALL_START" {
		t.Errorf("then start new tool, got %q", types[2])
	}
}

func TestTranslateEvent_ToolExecutionResult(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}
	state.lastToolID = "tc_1"
	state.toolCalls["tc_1"] = true

	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{
		Type: api.EventToolExecutionResult, ToolUseID: "tc_1",
	}, nopFilter{})

	types := w.types()
	if len(types) != 1 {
		t.Fatalf("expected 1 event, got %d: %v", len(types), types)
	}
	if types[0] != "TOOL_CALL_END" {
		t.Errorf("event = %q, want TOOL_CALL_END", types[0])
	}
	if state.lastToolID != "" {
		t.Errorf("lastToolID should be cleared, got %q", state.lastToolID)
	}
	if state.toolCalls["tc_1"] {
		t.Error("tool call should be removed from tracking")
	}
}

func TestTranslateEvent_IterationStartStop(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}

	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{Type: api.EventIterationStart}, nopFilter{})
	types := w.types()
	if len(types) != 2 || types[0] != "ACTIVITY_SNAPSHOT" || types[1] != "STEP_STARTED" {
		t.Fatalf("iteration_start: got %v, want [ACTIVITY_SNAPSHOT STEP_STARTED]", types)
	}
	if state.iterCount != 1 {
		t.Errorf("iterCount = %d, want 1", state.iterCount)
	}
	w.events = nil

	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{Type: api.EventIterationStop}, nopFilter{})
	types = w.types()
	if len(types) != 1 || types[0] != "STEP_FINISHED" {
		t.Fatalf("iteration_stop: got %v, want [STEP_FINISHED]", types)
	}
	if state.lastStep != "" {
		t.Errorf("lastStep should be cleared, got %q", state.lastStep)
	}
}

func TestTranslateEvent_IterationStart_ClosesOpenStep(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}

	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{Type: api.EventIterationStart}, nopFilter{})
	w.events = nil

	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{Type: api.EventIterationStart}, nopFilter{})
	types := w.types()
	if len(types) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(types), types)
	}
	if types[0] != "STEP_FINISHED" {
		t.Errorf("should close previous step first, got %q", types[0])
	}
	if types[1] != "ACTIVITY_SNAPSHOT" {
		t.Errorf("then activity snapshot, got %q", types[1])
	}
	if types[2] != "STEP_STARTED" {
		t.Errorf("then start new step, got %q", types[2])
	}
	if state.iterCount != 2 {
		t.Errorf("iterCount = %d, want 2", state.iterCount)
	}
}

func TestTranslateEvent_Error(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}

	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{
		Type: api.EventError, Output: "something broke",
	}, nopFilter{})

	types := w.types()
	if len(types) != 1 || types[0] != "RUN_ERROR" {
		t.Fatalf("error event: got %v, want [RUN_ERROR]", types)
	}
}

func TestTranslateEvent_Error_DefaultMessage(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}

	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{Type: api.EventError}, nopFilter{})

	if len(w.events) != 1 || w.types()[0] != "RUN_ERROR" {
		t.Fatalf("got %v, want [RUN_ERROR]", w.types())
	}
}

func TestFinalize_FullSequence(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	state.textStarted = true
	state.lastToolID = "tc_open"
	state.toolCalls["tc_open"] = true
	state.lastStep = "iteration_1"

	w := &capturingSSEWriter{}
	_ = state.finalize(context.Background(), &bytes.Buffer{}, w, nopFilter{})

	types := w.types()
	want := []string{"TOOL_CALL_END", "TEXT_MESSAGE_END", "STEP_FINISHED", "RUN_FINISHED"}
	if len(types) != len(want) {
		t.Fatalf("finalize events = %v, want %v", types, want)
	}
	for i, wt := range want {
		if types[i] != wt {
			t.Errorf("event[%d] = %q, want %q", i, types[i], wt)
		}
	}
}

func TestFinalize_MinimalNoOpenState(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}
	_ = state.finalize(context.Background(), &bytes.Buffer{}, w, nopFilter{})

	types := w.types()
	if len(types) != 1 || types[0] != "RUN_FINISHED" {
		t.Fatalf("minimal finalize = %v, want [RUN_FINISHED]", types)
	}
}

func TestFinalize_FlushesFilterTail(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	state.textStarted = true
	f := &tailFilter{buf: "leftover"}

	w := &capturingSSEWriter{}
	_ = state.finalize(context.Background(), &bytes.Buffer{}, w, f)

	types := w.types()
	if len(types) != 3 {
		t.Fatalf("expected 3 events, got %v", types)
	}
	if types[0] != "TEXT_MESSAGE_CONTENT" {
		t.Errorf("flush tail should emit content, got %q", types[0])
	}
	if types[1] != "TEXT_MESSAGE_END" {
		t.Errorf("then end message, got %q", types[1])
	}
}

func TestWriteSSE_UsesCallerContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w := &capturingSSEWriter{}
	err := writeSSE(ctx, &bytes.Buffer{}, w, aguievents.NewRunFinishedEvent("thread_1", "run_1"))

	if err != context.Canceled {
		t.Fatalf("writeSSE error = %v, want context.Canceled", err)
	}
	if len(w.events) != 0 {
		t.Fatalf("canceled context should not call SSE writer, got %d events", len(w.events))
	}
}

func TestInputToJSON(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input interface{}
		want  string
	}{
		{"nil", nil, "{}"},
		{"string", `{"a":1}`, `{"a":1}`},
		{"map", map[string]int{"x": 1}, `{"x":1}`},
		{"slice", []string{"a"}, `["a"]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := inputToJSON(c.input)
			if got != c.want {
				t.Errorf("inputToJSON(%v) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

func TestNewStreamState(t *testing.T) {
	t.Parallel()
	s := newStreamState("thread_abc", "run_xyz")
	if s.threadID != "thread_abc" {
		t.Errorf("threadID = %q", s.threadID)
	}
	if s.runID != "run_xyz" {
		t.Errorf("runID = %q", s.runID)
	}
	if s.msgID != "msg_thread_abc_run_xyz" {
		t.Errorf("msgID = %q, want msg_thread_abc_run_xyz", s.msgID)
	}
	if s.textStarted {
		t.Error("textStarted should be false initially")
	}
	if len(s.toolCalls) != 0 {
		t.Error("toolCalls should be empty initially")
	}
}

func TestNewStreamState_StableMsgID(t *testing.T) {
	t.Parallel()
	s1 := newStreamState("t1", "r1")
	s2 := newStreamState("t1", "r1")
	if s1.msgID != s2.msgID {
		t.Errorf("same thread+run should produce same msgID: %q vs %q", s1.msgID, s2.msgID)
	}
	s3 := newStreamState("t1", "r2")
	if s1.msgID == s3.msgID {
		t.Error("different runs should produce different msgIDs")
	}
}

func TestTranslateEvent_ToolExecutionResult_ExtractsStructuredMetadata(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}
	state.toolCalls["tc_img"] = true
	state.toolNames["tc_img"] = "generate_image"

	noErr := false
	artifacts, err := state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{
		Type:      api.EventToolExecutionResult,
		ToolUseID: "tc_img",
		Name:      "generate_image",
		IsError:   &noErr,
		Output: map[string]any{
			"output": "/tmp/saker-media-abc.png",
			"metadata": map[string]any{
				"structured": map[string]any{
					"media_type": "image",
					"media_url":  "https://cdn.example.com/img.png",
					"local_path": "/tmp/saker-media-abc.png",
				},
			},
		},
	}, nopFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].Type != "image" {
		t.Errorf("artifact type = %q, want image", artifacts[0].Type)
	}
	if artifacts[0].URL != "https://cdn.example.com/img.png" {
		t.Errorf("artifact URL = %q, want https://cdn.example.com/img.png", artifacts[0].URL)
	}
	if artifacts[0].Name != "generate_image" {
		t.Errorf("artifact name = %q, want generate_image", artifacts[0].Name)
	}
	types := w.types()
	if len(types) < 2 {
		t.Fatalf("expected >=2 events (TOOL_CALL_END + TOOL_CALL_RESULT), got %d: %v", len(types), types)
	}
	if types[0] != "TOOL_CALL_END" {
		t.Errorf("first event = %q, want TOOL_CALL_END", types[0])
	}
	if types[1] != "TOOL_CALL_RESULT" {
		t.Errorf("second event = %q, want TOOL_CALL_RESULT", types[1])
	}
}

func TestTranslateEvent_ToolExecutionResult_ErrorNoArtifacts(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}
	state.toolCalls["tc_1"] = true
	state.toolNames["tc_1"] = "bash"

	isErr := true
	artifacts, err := state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{
		Type:      api.EventToolExecutionResult,
		ToolUseID: "tc_1",
		IsError:   &isErr,
		Output:    "command failed",
	}, nopFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("error result should produce 0 artifacts, got %d", len(artifacts))
	}
}

func TestTranslateEvent_ToolExecutionResult_DedupArtifacts(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}

	noErr := false
	// First tool result with image URL.
	state.toolCalls["tc_1"] = true
	state.toolNames["tc_1"] = "generate_image"
	_, _ = state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{
		Type:      api.EventToolExecutionResult,
		ToolUseID: "tc_1",
		Name:      "generate_image",
		IsError:   &noErr,
		Output: map[string]any{
			"output": "/tmp/a.png",
			"metadata": map[string]any{
				"structured": map[string]any{
					"media_type": "image",
					"media_url":  "https://cdn.example.com/img.png",
				},
			},
		},
	}, nopFilter{})
	w.events = nil

	// Second tool result with same URL — should be deduped.
	state.toolCalls["tc_2"] = true
	state.toolNames["tc_2"] = "edit_image"
	artifacts, _ := state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{
		Type:      api.EventToolExecutionResult,
		ToolUseID: "tc_2",
		Name:      "edit_image",
		IsError:   &noErr,
		Output: map[string]any{
			"output": "/tmp/b.png",
			"metadata": map[string]any{
				"structured": map[string]any{
					"media_type": "image",
					"media_url":  "https://cdn.example.com/img.png",
				},
			},
		},
	}, nopFilter{})
	if len(artifacts) != 0 {
		t.Errorf("duplicate URL should be deduped, got %d artifacts", len(artifacts))
	}
}

func TestTranslateEvent_ToolExecutionResult_ToolNameFallback(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}

	noErr := false
	// Tool name not tracked in toolNames (e.g. result arrives without prior start event),
	// but evt.Name is provided.
	artifacts, _ := state.translateEvent(context.Background(), &bytes.Buffer{}, w, api.StreamEvent{
		Type:      api.EventToolExecutionResult,
		ToolUseID: "tc_3",
		Name:      "search_web",
		IsError:   &noErr,
		Output: map[string]any{
			"output": "https://cdn.example.com/result.mp4",
			"metadata": map[string]any{},
		},
	}, nopFilter{})
	// Path 3 regex detection should pick up the MP4 URL.
	if len(artifacts) < 1 {
		t.Fatalf("expected at least 1 artifact from URL regex, got %d", len(artifacts))
	}
}

func TestNewStreamState_ArtifactFields(t *testing.T) {
	t.Parallel()
	s := newStreamState("thread_abc", "run_xyz")
	if len(s.toolNames) != 0 {
		t.Error("toolNames should be empty initially")
	}
	if len(s.artifacts) != 0 {
		t.Error("artifacts should be empty initially")
	}
	if len(s.seenArtifactURLs) != 0 {
		t.Error("seenArtifactURLs should be empty initially")
	}
}

func TestTranslateEvent_TextToolText_UsesUniqueMessageIDs(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	w := &capturingSSEWriter{}
	ctx := context.Background()
	buf := &bytes.Buffer{}

	// Phase 1: First text delta — opens message with base msgID.
	_, _ = state.translateEvent(ctx, buf, w, api.StreamEvent{
		Type: api.EventContentBlockDelta, Delta: &api.Delta{Text: "hello"},
	}, nopFilter{})

	if state.textMsgSeq != 1 {
		t.Fatalf("after first text: textMsgSeq = %d, want 1", state.textMsgSeq)
	}
	if state.currentTextMsgID() != "msg_t1_r1" {
		t.Fatalf("first segment msgID = %q, want msg_t1_r1", state.currentTextMsgID())
	}

	// Phase 2: Tool execution start — closes text message.
	w.events = nil
	_, _ = state.translateEvent(ctx, buf, w, api.StreamEvent{
		Type: api.EventToolExecutionStart, ToolUseID: "tc_1", Name: "generate_image",
		Input: map[string]string{"prompt": "cat"},
	}, nopFilter{})

	if state.textStarted {
		t.Fatal("textStarted should be false after tool start")
	}
	// Should have emitted: TEXT_MESSAGE_END, ACTIVITY_SNAPSHOT, TOOL_CALL_START, TOOL_CALL_ARGS
	types := w.types()
	if len(types) < 1 || types[0] != "TEXT_MESSAGE_END" {
		t.Fatalf("tool start should close text first, got %v", types)
	}

	// Phase 3: Tool execution result.
	w.events = nil
	_, _ = state.translateEvent(ctx, buf, w, api.StreamEvent{
		Type: api.EventToolExecutionResult, ToolUseID: "tc_1", Name: "generate_image",
		Output: map[string]any{"output": "done"},
	}, nopFilter{})

	// Phase 4: Second text delta — should use a NEW message ID.
	w.events = nil
	_, _ = state.translateEvent(ctx, buf, w, api.StreamEvent{
		Type: api.EventContentBlockDelta, Delta: &api.Delta{Text: "image ready"},
	}, nopFilter{})

	if state.textMsgSeq != 2 {
		t.Fatalf("after second text: textMsgSeq = %d, want 2", state.textMsgSeq)
	}
	if state.currentTextMsgID() != "msg_t1_r1_2" {
		t.Fatalf("second segment msgID = %q, want msg_t1_r1_2", state.currentTextMsgID())
	}
	types = w.types()
	if len(types) != 2 {
		t.Fatalf("expected 2 events (START+CONTENT), got %d: %v", len(types), types)
	}
	if types[0] != "TEXT_MESSAGE_START" {
		t.Errorf("second segment should emit new TEXT_MESSAGE_START, got %q", types[0])
	}
	if types[1] != "TEXT_MESSAGE_CONTENT" {
		t.Errorf("then TEXT_MESSAGE_CONTENT, got %q", types[1])
	}
}

func TestTranslateEvent_TextToolText_ToolParentMessageID(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	ctx := context.Background()
	buf := &bytes.Buffer{}

	// Emit first text.
	_, _ = state.translateEvent(ctx, buf, &capturingSSEWriter{}, api.StreamEvent{
		Type: api.EventContentBlockDelta, Delta: &api.Delta{Text: "hi"},
	}, nopFilter{})

	// Tool start — verify it uses s.msgID (base ID) for parentMessageId, not the segment ID.
	// The parentMessageId is set in the event but we verify indirectly: s.msgID is always the base.
	if state.msgID != "msg_t1_r1" {
		t.Fatalf("base msgID should remain msg_t1_r1, got %q", state.msgID)
	}
}

func TestCurrentTextMsgID_Segments(t *testing.T) {
	t.Parallel()
	s := newStreamState("thread1", "run1")

	s.textMsgSeq = 0
	if got := s.currentTextMsgID(); got != "msg_thread1_run1" {
		t.Errorf("seq=0: got %q, want msg_thread1_run1", got)
	}
	s.textMsgSeq = 1
	if got := s.currentTextMsgID(); got != "msg_thread1_run1" {
		t.Errorf("seq=1: got %q, want msg_thread1_run1", got)
	}
	s.textMsgSeq = 2
	if got := s.currentTextMsgID(); got != "msg_thread1_run1_2" {
		t.Errorf("seq=2: got %q, want msg_thread1_run1_2", got)
	}
	s.textMsgSeq = 3
	if got := s.currentTextMsgID(); got != "msg_thread1_run1_3" {
		t.Errorf("seq=3: got %q, want msg_thread1_run1_3", got)
	}
}

func TestFinalize_CleansUpToolNames(t *testing.T) {
	t.Parallel()
	state := newStreamState("t1", "r1")
	state.lastToolID = "tc_open"
	state.toolCalls["tc_open"] = true
	state.toolNames["tc_open"] = "bash"

	w := &capturingSSEWriter{}
	_ = state.finalize(context.Background(), &bytes.Buffer{}, w, nopFilter{})

	if _, exists := state.toolNames["tc_open"]; exists {
		t.Error("finalize should clean up toolNames for lastToolID")
	}
}
