package agui

import (
	"encoding/json"
	"testing"
)

func TestActionEventsUseToolCallLifecycle(t *testing.T) {
	events := actionEvents("question_request", "question_123", map[string]any{
		"question_id": "123",
		"run_id":      "run_1",
	})

	if len(events) != 3 {
		t.Fatalf("expected 3 action events, got %d", len(events))
	}
	want := []string{"TOOL_CALL_START", "TOOL_CALL_ARGS", "TOOL_CALL_END"}
	for i, event := range events {
		if string(event.Type()) != want[i] {
			t.Fatalf("event[%d] type = %s, want %s", i, event.Type(), want[i])
		}
	}

	raw, err := events[1].ToJSON()
	if err != nil {
		t.Fatalf("args ToJSON: %v", err)
	}
	var got struct {
		Delta string `json:"delta"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal args: %v", err)
	}
	if got.Delta == "" || !json.Valid([]byte(got.Delta)) {
		t.Fatalf("TOOL_CALL_ARGS delta should be JSON object, got %q", got.Delta)
	}
}
