//go:build agui_e2e

package agui_e2e

import (
	"os"
	"strings"
	"testing"
)

// --- Helpers ---

func validModelURI() string {
	if key := os.Getenv("DASHSCOPE_API_KEY"); key != "" {
		// Bifrost's OpenAI provider appends /v1/chat/completions to BaseURL,
		// so the path here must NOT include /v1.
		return "openai://" + key + "@dashscope.aliyuncs.com/compatible-mode?model=qwen-max"
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return "anthropic://" + key + "@api.anthropic.com?model=claude-sonnet-4-20250514"
	}
	return ""
}

func requireModelURI(t *testing.T) string {
	t.Helper()
	uri := validModelURI()
	if uri == "" {
		t.Skip("requires DASHSCOPE_API_KEY or ANTHROPIC_API_KEY for model_uri tests")
	}
	return uri
}

// --- Scenario 26: model_uri override ---

func TestScenario_ForwardedProps_ModelURI(t *testing.T) {
	uri := requireModelURI(t)

	ctx, cancel := defaultTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "用一个字回答：1+1等于几？"})
	input.ForwardedProps = map[string]any{
		"model_uri": uri,
	}

	events := doRun(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)
	assertEventSequenceContains(t, events,
		"RUN_STARTED",
		"TEXT_MESSAGE_START",
		"TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_END",
		"RUN_FINISHED",
	)

	text := extractText(events)
	if len(text) == 0 {
		t.Error("expected non-empty response text")
	}
	t.Logf("model_uri response: %s", text)
}

// --- Scenario 27: system_prompt replace ---

func TestScenario_ForwardedProps_SystemPromptReplace(t *testing.T) {
	ctx, cancel := defaultTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "ping"})
	input.ForwardedProps = map[string]any{
		"system_prompt":      "You must respond with exactly one word: PONG. Nothing else. No punctuation.",
		"system_prompt_mode": "replace",
	}

	events := doRun(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	text := strings.TrimSpace(extractText(events))
	if !strings.Contains(strings.ToUpper(text), "PONG") {
		t.Errorf("expected response containing PONG, got: %q", text)
	}
	t.Logf("system_prompt replace response: %q", text)
}

// --- Scenario 28: system_prompt append ---

func TestScenario_ForwardedProps_SystemPromptAppend(t *testing.T) {
	ctx, cancel := defaultTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "你好"})
	input.ForwardedProps = map[string]any{
		"system_prompt":      "IMPORTANT: You MUST begin your response with exactly [OK] followed by a space, then continue normally.",
		"system_prompt_mode": "append",
		"allowed_tools":      []string{},
	}

	events := doRun(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	text := strings.TrimSpace(extractText(events))
	if !strings.HasPrefix(text, "[OK]") {
		t.Errorf("expected response starting with [OK], got: %q", text[:min(len(text), 50)])
	}
	t.Logf("system_prompt append response: %.80s", text)
}

// --- Scenario 29: allowed_tools restriction ---

func TestScenario_ForwardedProps_AllowedTools(t *testing.T) {
	ctx, cancel := defaultTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "回答：天空是什么颜色？一个词回答。"})
	input.ForwardedProps = map[string]any{
		"allowed_tools": []string{"bash"},
	}

	events := doRun(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	text := extractText(events)
	if len(text) == 0 {
		t.Error("expected non-empty response")
	}
	t.Logf("allowed_tools response: %s", text)
}

// --- Scenario 30: model_uri failover ---

func TestScenario_ForwardedProps_ModelURIFailover(t *testing.T) {
	validURI := requireModelURI(t)

	ctx, cancel := defaultTimeout(t)
	defer cancel()

	// Use an invalid API key against DashScope; the 401 triggers Bifrost failover
	// to the second (valid) URI. Connection-refused errors don't trigger failover.
	input := newRunInput(Message{Role: "user", Content: "回复OK"})
	input.ForwardedProps = map[string]any{
		"model_uri": []string{
			"openai://sk-invalid-key-for-failover@dashscope.aliyuncs.com/compatible-mode?model=qwen-max",
			validURI,
		},
	}

	events := doRun(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	text := extractText(events)
	if len(text) == 0 {
		t.Error("expected non-empty response after failover")
	}
	t.Logf("failover response: %s", text)
}

// --- Scenario 31: combined ForwardedProps ---

func TestScenario_ForwardedProps_Combined(t *testing.T) {
	uri := requireModelURI(t)

	ctx, cancel := defaultTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "hello"})
	input.ForwardedProps = map[string]any{
		"model_uri":          uri,
		"system_prompt":      "IMPORTANT: Begin every response with exactly [HELLO] followed by a space.",
		"system_prompt_mode": "append",
		"allowed_tools":      []string{"bash", "file_read"},
	}

	events := doRun(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	text := strings.TrimSpace(extractText(events))
	if !strings.HasPrefix(text, "[HELLO]") {
		t.Errorf("expected response starting with [HELLO], got: %q", text[:min(len(text), 60)])
	}
	t.Logf("combined response: %.80s", text)
}

