package agui

import (
	"testing"

	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

func TestMessagesToRequest_SimpleUser(t *testing.T) {
	t.Parallel()
	input := aguitypes.RunAgentInput{
		ThreadID: "thread_1",
		Messages: []aguitypes.Message{
			{Role: aguitypes.RoleUser, Content: "Hello saker"},
		},
	}
	req := messagesToRequest(input, Identity{})
	if req.Prompt != "Hello saker" {
		t.Errorf("Prompt = %q, want %q", req.Prompt, "Hello saker")
	}
	if req.SessionID != "thread_1" {
		t.Errorf("SessionID = %q, want thread_1", req.SessionID)
	}
	if !req.Ephemeral {
		t.Error("Ephemeral should be true so AG-UI does not double-persist runtime history")
	}
}

func TestMessagesToRequest_LastUserMessage(t *testing.T) {
	t.Parallel()
	input := aguitypes.RunAgentInput{
		ThreadID: "t1",
		Messages: []aguitypes.Message{
			{Role: aguitypes.RoleUser, Content: "first"},
			{Role: aguitypes.RoleAssistant, Content: "ack"},
			{Role: aguitypes.RoleUser, Content: "second"},
		},
	}
	req := messagesToRequest(input, Identity{})
	if req.Prompt != "second" {
		t.Errorf("Prompt = %q, want last user message %q", req.Prompt, "second")
	}
}

func TestMessagesToRequest_NoUserMessage(t *testing.T) {
	t.Parallel()
	input := aguitypes.RunAgentInput{
		ThreadID: "t1",
		Messages: []aguitypes.Message{
			{Role: aguitypes.RoleAssistant, Content: "hello"},
		},
	}
	req := messagesToRequest(input, Identity{})
	if req.Prompt != "" {
		t.Errorf("Prompt should be empty when no user message, got %q", req.Prompt)
	}
}

func TestMessagesToRequest_ContextInjection(t *testing.T) {
	t.Parallel()
	input := aguitypes.RunAgentInput{
		ThreadID: "t1",
		Messages: []aguitypes.Message{
			{Role: aguitypes.RoleUser, Content: "do it"},
		},
		Context: []aguitypes.Context{
			{Description: "Project", Value: "saker"},
			{Description: "Language", Value: "Go"},
		},
	}
	req := messagesToRequest(input, Identity{})
	if req.Prompt == "do it" {
		t.Error("context should be prepended to prompt")
	}
	if got := req.Prompt; got == "" {
		t.Fatal("prompt should not be empty")
	}
	if !contains(req.Prompt, "Project: saker") {
		t.Errorf("prompt should contain context, got %q", req.Prompt)
	}
	if !contains(req.Prompt, "Language: Go") {
		t.Errorf("prompt should contain context, got %q", req.Prompt)
	}
	if !contains(req.Prompt, "do it") {
		t.Errorf("prompt should still contain user message, got %q", req.Prompt)
	}
}

func TestMessagesToRequest_IdentityPropagation(t *testing.T) {
	t.Parallel()
	input := aguitypes.RunAgentInput{
		ThreadID: "t1",
		Messages: []aguitypes.Message{
			{Role: aguitypes.RoleUser, Content: "hi"},
		},
	}
	id := Identity{Username: "alice", UserID: "u_1"}
	req := messagesToRequest(input, id)
	if req.User != "alice" {
		t.Errorf("User = %q, want alice", req.User)
	}
}

func TestMessagesToRequest_EmptyIdentity(t *testing.T) {
	t.Parallel()
	input := aguitypes.RunAgentInput{
		ThreadID: "t1",
		Messages: []aguitypes.Message{
			{Role: aguitypes.RoleUser, Content: "hi"},
		},
	}
	req := messagesToRequest(input, Identity{})
	if req.User != "" {
		t.Errorf("User should be empty, got %q", req.User)
	}
}

func TestExtractTextContent_String(t *testing.T) {
	t.Parallel()
	if got := extractTextContent("hello"); got != "hello" {
		t.Errorf("string content: got %q, want hello", got)
	}
}

func TestExtractTextContent_Nil(t *testing.T) {
	t.Parallel()
	if got := extractTextContent(nil); got != "" {
		t.Errorf("nil content: got %q, want empty", got)
	}
}

func TestExtractTextContent_TextParts(t *testing.T) {
	t.Parallel()
	parts := []map[string]string{
		{"type": "text", "text": "part1"},
		{"type": "text", "text": "part2"},
	}
	got := extractTextContent(parts)
	if !contains(got, "part1") || !contains(got, "part2") {
		t.Errorf("text parts: got %q, want both parts", got)
	}
}

func TestExtractTextContent_NonTextPartsIgnored(t *testing.T) {
	t.Parallel()
	parts := []map[string]string{
		{"type": "image", "text": ""},
		{"type": "text", "text": "visible"},
	}
	got := extractTextContent(parts)
	if !contains(got, "visible") {
		t.Errorf("got %q, want 'visible'", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestMessagesToRequest_FrontendTools(t *testing.T) {
	t.Parallel()
	input := aguitypes.RunAgentInput{
		ThreadID: "t1",
		Messages: []aguitypes.Message{
			{Role: aguitypes.RoleUser, Content: "hi"},
		},
		Tools: []aguitypes.Tool{
			{Name: "searchWeb", Description: "Search the web", Parameters: map[string]any{"type": "object"}},
			{Name: "navigate", Description: "Go to URL"},
		},
	}
	req := messagesToRequest(input, Identity{})
	if len(req.ExtraTools) != 2 {
		t.Fatalf("ExtraTools = %d, want 2", len(req.ExtraTools))
	}
	if req.ExtraTools[0].Name != "searchWeb" {
		t.Errorf("ExtraTools[0].Name = %q, want searchWeb", req.ExtraTools[0].Name)
	}
	if req.ExtraTools[1].Description != "Go to URL" {
		t.Errorf("ExtraTools[1].Description = %q", req.ExtraTools[1].Description)
	}
	// Frontend tools must be passthrough.
	if len(req.PassthroughTools) != 2 {
		t.Fatalf("PassthroughTools = %d, want 2", len(req.PassthroughTools))
	}
	if req.PassthroughTools[0] != "searchWeb" || req.PassthroughTools[1] != "navigate" {
		t.Errorf("PassthroughTools = %v", req.PassthroughTools)
	}
}

func TestMessagesToRequest_ParentRunID(t *testing.T) {
	t.Parallel()
	parentID := "run_parent_123"
	input := aguitypes.RunAgentInput{
		ThreadID:    "t1",
		ParentRunID: &parentID,
		Messages:    []aguitypes.Message{{Role: aguitypes.RoleUser, Content: "hi"}},
	}
	req := messagesToRequest(input, Identity{})
	if req.ParentSessionID != "run_parent_123" {
		t.Errorf("ParentSessionID = %q, want run_parent_123", req.ParentSessionID)
	}
}

func TestMessagesToRequest_StateForwarding(t *testing.T) {
	t.Parallel()
	state := map[string]any{"count": float64(3), "items": []any{"a", "b"}}
	input := aguitypes.RunAgentInput{
		ThreadID: "t1",
		Messages: []aguitypes.Message{
			{Role: aguitypes.RoleUser, Content: "hi"},
		},
		State: state,
	}
	req := messagesToRequest(input, Identity{})
	if req.Metadata == nil {
		t.Fatal("Metadata should not be nil")
	}
	got, ok := req.Metadata["_agui_state"]
	if !ok {
		t.Fatal("_agui_state not found in Metadata")
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("_agui_state type = %T, want map[string]any", got)
	}
	if m["count"] != float64(3) {
		t.Errorf("state.count = %v", m["count"])
	}
}

func TestConvertFrontendTools_NilParameters(t *testing.T) {
	t.Parallel()
	tools := []aguitypes.Tool{
		{Name: "myTool", Description: "does stuff", Parameters: nil},
	}
	out := convertFrontendTools(tools)
	if len(out) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(out))
	}
	if out[0].Parameters != nil {
		t.Errorf("Parameters should be nil for nil input, got %v", out[0].Parameters)
	}
}

func TestMessagesToRequest_PreloadHistory(t *testing.T) {
	t.Parallel()
	input := aguitypes.RunAgentInput{
		ThreadID: "thread_restore",
		Messages: []aguitypes.Message{
			{Role: aguitypes.RoleUser, Content: "hello"},
			{Role: aguitypes.RoleAssistant, Content: "hi there"},
			{Role: aguitypes.RoleUser, Content: "what is 2+2?"},
		},
	}
	req := messagesToRequest(input, Identity{})

	if req.Prompt != "what is 2+2?" {
		t.Errorf("Prompt = %q, want %q", req.Prompt, "what is 2+2?")
	}
	if len(req.PreloadHistory) != 2 {
		t.Fatalf("PreloadHistory len = %d, want 2", len(req.PreloadHistory))
	}
	if req.PreloadHistory[0].Role != "user" || req.PreloadHistory[0].Content != "hello" {
		t.Errorf("PreloadHistory[0] = %+v, want user/hello", req.PreloadHistory[0])
	}
	if req.PreloadHistory[1].Role != "assistant" || req.PreloadHistory[1].Content != "hi there" {
		t.Errorf("PreloadHistory[1] = %+v, want assistant/hi there", req.PreloadHistory[1])
	}
}

func TestMessagesToRequest_PreloadHistoryWithToolCalls(t *testing.T) {
	t.Parallel()
	input := aguitypes.RunAgentInput{
		ThreadID: "thread_tools",
		Messages: []aguitypes.Message{
			{Role: aguitypes.RoleUser, Content: "search for cats"},
			{
				Role:    aguitypes.RoleAssistant,
				Content: "",
				ToolCalls: []aguitypes.ToolCall{
					{ID: "call_1", Type: "function", Function: aguitypes.FunctionCall{
						Name:      "web_search",
						Arguments: `{"query":"cats"}`,
					}},
				},
			},
			{Role: aguitypes.RoleTool, Content: "found 10 results", ToolCallID: "call_1"},
			{Role: aguitypes.RoleAssistant, Content: "I found 10 results about cats"},
			{Role: aguitypes.RoleUser, Content: "tell me more"},
		},
	}
	req := messagesToRequest(input, Identity{})

	if req.Prompt != "tell me more" {
		t.Errorf("Prompt = %q, want %q", req.Prompt, "tell me more")
	}
	if len(req.PreloadHistory) != 4 {
		t.Fatalf("PreloadHistory len = %d, want 4", len(req.PreloadHistory))
	}
	// Assistant with tool call
	if len(req.PreloadHistory[1].ToolCalls) != 1 {
		t.Fatalf("PreloadHistory[1].ToolCalls len = %d, want 1", len(req.PreloadHistory[1].ToolCalls))
	}
	tc := req.PreloadHistory[1].ToolCalls[0]
	if tc.Name != "web_search" || tc.ID != "call_1" {
		t.Errorf("ToolCall = %+v, want web_search/call_1", tc)
	}
	if tc.Arguments["query"] != "cats" {
		t.Errorf("ToolCall args = %v, want query=cats", tc.Arguments)
	}
	// Tool result
	if req.PreloadHistory[2].Role != "tool" {
		t.Errorf("PreloadHistory[2].Role = %q, want tool", req.PreloadHistory[2].Role)
	}
	if len(req.PreloadHistory[2].ToolCalls) != 1 || req.PreloadHistory[2].ToolCalls[0].Result != "found 10 results" {
		t.Errorf("PreloadHistory[2] tool result mismatch: %+v", req.PreloadHistory[2])
	}
}

func TestMessagesToRequest_SingleMessageNoPreload(t *testing.T) {
	t.Parallel()
	input := aguitypes.RunAgentInput{
		ThreadID: "thread_new",
		Messages: []aguitypes.Message{
			{Role: aguitypes.RoleUser, Content: "first message"},
		},
	}
	req := messagesToRequest(input, Identity{})

	if req.Prompt != "first message" {
		t.Errorf("Prompt = %q, want %q", req.Prompt, "first message")
	}
	if len(req.PreloadHistory) != 0 {
		t.Errorf("PreloadHistory should be empty for single message, got %d", len(req.PreloadHistory))
	}
}
