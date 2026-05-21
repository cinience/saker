//go:build agui_e2e

package agui_e2e

import (
	"os"
	"strings"
	"testing"
)

// --- Helpers ---

// dashscopeKey returns the DashScope API key from available env vars.
func dashscopeKey() string {
	for _, env := range []string{"DASHSCOPE_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY"} {
		if key := os.Getenv(env); key != "" {
			return key
		}
	}
	return ""
}

// validModelURI returns an openai-protocol DashScope model_uri.
// Bifrost's OpenAI provider appends /v1/chat/completions to BaseURL.
func validModelURI() string {
	if key := dashscopeKey(); key != "" {
		return "openai://" + key + "@dashscope.aliyuncs.com/compatible-mode?model=qwen-max"
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return "openai://" + key + "@api.openai.com?model=gpt-4o-mini"
	}
	return ""
}

// validModelURIAnthropic returns an anthropic-protocol DashScope model_uri.
// Bifrost's Anthropic provider appends /v1/messages to BaseURL.
func validModelURIAnthropic() string {
	if key := dashscopeKey(); key != "" {
		return "anthropic://" + key + "@dashscope.aliyuncs.com/apps/anthropic?model=qwen3.7-max"
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
		t.Skip("requires DASHSCOPE_API_KEY, ANTHROPIC_AUTH_TOKEN, or ANTHROPIC_API_KEY")
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
	uri := requireModelURI(t)

	ctx, cancel := defaultTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "你好"})
	input.ForwardedProps = map[string]any{
		"model_uri":          uri,
		"system_prompt":      "IMPORTANT: You MUST begin your response with exactly [OK] followed by a space, then continue normally.",
		"system_prompt_mode": "append",
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

	// Cross-provider failover: invalid anthropic key triggers 401, then Bifrost
	// falls back to the valid openai URI. Same-provider fallback shares the
	// primary's key, so we use different providers for cross-provider failover.
	input := newRunInput(Message{Role: "user", Content: "回复OK"})
	input.ForwardedProps = map[string]any{
		"model_uri": []string{
			"anthropic://sk-invalid-key@dashscope.aliyuncs.com/apps/anthropic?model=qwen3.7-max",
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

// --- Scenario 31: model_uri with anthropic protocol (DashScope compatible) ---

func TestScenario_ForwardedProps_ModelURIAnthropic(t *testing.T) {
	uri := validModelURIAnthropic()
	if uri == "" {
		t.Skip("requires DashScope or Anthropic key for anthropic-protocol model_uri test")
	}

	ctx, cancel := defaultTimeout(t)
	defer cancel()

	input := newRunInput(Message{Role: "user", Content: "用一个字回答：1+1等于几？"})
	input.ForwardedProps = map[string]any{
		"model_uri": uri,
	}

	events := doRun(t, ctx, input)

	assertRunLifecycle(t, events)
	assertNoRunError(t, events)

	text := extractText(events)
	if len(text) == 0 {
		t.Error("expected non-empty response text")
	}
	t.Logf("model_uri (anthropic protocol) response: %s", text)
}

// --- Scenario 32: combined ForwardedProps ---

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

