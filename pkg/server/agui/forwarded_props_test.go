package agui

import (
	"testing"

	"github.com/saker-ai/saker/pkg/api"
)

func TestParseModelURI_Valid(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		wantProv    string
		wantModel   string
		wantKey     string
		wantBaseURL string
	}{
		{
			name:        "openai with path",
			uri:         "openai://sk-test123@api.openai.com/v1?model=gpt-4o",
			wantProv:    "openai",
			wantModel:   "gpt-4o",
			wantKey:     "sk-test123",
			wantBaseURL: "https://api.openai.com/v1",
		},
		{
			name:        "anthropic no path",
			uri:         "anthropic://sk-ant-xxx@api.anthropic.com?model=claude-sonnet-4-20250514",
			wantProv:    "anthropic",
			wantModel:   "claude-sonnet-4-20250514",
			wantKey:     "sk-ant-xxx",
			wantBaseURL: "https://api.anthropic.com",
		},
		{
			name:        "localhost uses http",
			uri:         "openai://ollama@localhost:11434/v1?model=llama3",
			wantProv:    "openai",
			wantModel:   "llama3",
			wantKey:     "ollama",
			wantBaseURL: "http://localhost:11434/v1",
		},
		{
			name:        "127.0.0.1 uses http",
			uri:         "openai://key@127.0.0.1:8080/v1?model=test",
			wantProv:    "openai",
			wantModel:   "test",
			wantKey:     "key",
			wantBaseURL: "http://127.0.0.1:8080/v1",
		},
		{
			name:        "dashscope",
			uri:         "dashscope://sk-xxx@dashscope.aliyuncs.com/compatible-mode/v1?model=qwen-max",
			wantProv:    "dashscope",
			wantModel:   "qwen-max",
			wantKey:     "sk-xxx",
			wantBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry, _, err := parseModelURI(tc.uri)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if entry.Provider != tc.wantProv {
				t.Errorf("provider = %q, want %q", entry.Provider, tc.wantProv)
			}
			if entry.Model != tc.wantModel {
				t.Errorf("model = %q, want %q", entry.Model, tc.wantModel)
			}
			if entry.APIKey != tc.wantKey {
				t.Errorf("apiKey = %q, want %q", entry.APIKey, tc.wantKey)
			}
			if entry.BaseURL != tc.wantBaseURL {
				t.Errorf("baseURL = %q, want %q", entry.BaseURL, tc.wantBaseURL)
			}
		})
	}
}

func TestParseModelURI_Overrides(t *testing.T) {
	uri := "openai://sk@api.openai.com/v1?model=gpt-4o&temperature=0.7&top_p=0.9&max_tokens=4096&stop=END,STOP&seed=42&tool_choice=auto&parallel_tool_calls=true"

	_, ov, err := parseModelURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ov == nil {
		t.Fatal("expected non-nil overrides")
	}
	if ov.Temperature == nil || *ov.Temperature != 0.7 {
		t.Errorf("temperature = %v, want 0.7", ov.Temperature)
	}
	if ov.TopP == nil || *ov.TopP != 0.9 {
		t.Errorf("top_p = %v, want 0.9", ov.TopP)
	}
	if ov.MaxTokens == nil || *ov.MaxTokens != 4096 {
		t.Errorf("max_tokens = %v, want 4096", ov.MaxTokens)
	}
	if len(ov.Stop) != 2 || ov.Stop[0] != "END" || ov.Stop[1] != "STOP" {
		t.Errorf("stop = %v, want [END STOP]", ov.Stop)
	}
	if ov.Seed == nil || *ov.Seed != 42 {
		t.Errorf("seed = %v, want 42", ov.Seed)
	}
	if ov.ToolChoice != "auto" {
		t.Errorf("tool_choice = %q, want %q", ov.ToolChoice, "auto")
	}
	if ov.ParallelToolCalls == nil || *ov.ParallelToolCalls != true {
		t.Errorf("parallel_tool_calls = %v, want true", ov.ParallelToolCalls)
	}
}

func TestParseModelURI_NoOverrides(t *testing.T) {
	uri := "openai://sk@api.openai.com/v1?model=gpt-4o"
	_, ov, err := parseModelURI(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ov != nil {
		t.Errorf("expected nil overrides, got %+v", ov)
	}
}

func TestParseModelURI_Invalid(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{"missing scheme", "://sk@api.openai.com?model=x"},
		{"unsupported provider", "gemini://sk@api.google.com?model=x"},
		{"missing model param", "openai://sk@api.openai.com/v1"},
		{"empty string", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseModelURI(tc.uri)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestApplyForwardedProps_Empty(t *testing.T) {
	req := api.Request{}
	err := applyForwardedProps(map[string]any{}, &req, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.ModelEndpoint != nil {
		t.Error("expected nil ModelEndpoint")
	}
	if req.SystemPromptOverride != nil {
		t.Error("expected nil SystemPromptOverride")
	}
}

func TestApplyForwardedProps_ModelURI(t *testing.T) {
	props := map[string]any{
		"model_uri": "openai://sk-key@api.openai.com/v1?model=gpt-4o&temperature=0.5",
	}
	req := api.Request{}
	err := applyForwardedProps(props, &req, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.ModelEndpoint == nil {
		t.Fatal("expected non-nil ModelEndpoint")
	}
	if req.ModelEndpoint.Provider != "openai" {
		t.Errorf("provider = %q, want openai", req.ModelEndpoint.Provider)
	}
	if req.ModelEndpoint.Model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", req.ModelEndpoint.Model)
	}
	if req.ModelOverrides == nil || req.ModelOverrides.Temperature == nil {
		t.Fatal("expected ModelOverrides with temperature")
	}
	if *req.ModelOverrides.Temperature != 0.5 {
		t.Errorf("temperature = %v, want 0.5", *req.ModelOverrides.Temperature)
	}
}

func TestApplyForwardedProps_DenyModelEndpoint(t *testing.T) {
	props := map[string]any{
		"model_uri": "openai://sk@api.openai.com/v1?model=gpt-4o",
	}
	req := api.Request{}
	err := applyForwardedProps(props, &req, Options{DenyModelEndpoint: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.ModelEndpoint != nil {
		t.Error("expected nil ModelEndpoint when DenyModelEndpoint=true")
	}
}

func TestApplyForwardedProps_SystemPrompt(t *testing.T) {
	tests := []struct {
		name     string
		props    map[string]any
		wantMode api.SystemPromptMode
	}{
		{
			name:     "default mode is prepend",
			props:    map[string]any{"system_prompt": "hello"},
			wantMode: api.SystemPromptModePrepend,
		},
		{
			name:     "explicit append",
			props:    map[string]any{"system_prompt": "hello", "system_prompt_mode": "append"},
			wantMode: api.SystemPromptModeAppend,
		},
		{
			name:     "explicit replace",
			props:    map[string]any{"system_prompt": "hello", "system_prompt_mode": "replace"},
			wantMode: api.SystemPromptModeReplace,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := api.Request{}
			err := applyForwardedProps(tc.props, &req, Options{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if req.SystemPromptOverride == nil {
				t.Fatal("expected non-nil SystemPromptOverride")
			}
			if req.SystemPromptOverride.Text != "hello" {
				t.Errorf("text = %q, want %q", req.SystemPromptOverride.Text, "hello")
			}
			if req.SystemPromptOverride.Mode != tc.wantMode {
				t.Errorf("mode = %q, want %q", req.SystemPromptOverride.Mode, tc.wantMode)
			}
		})
	}
}

func TestApplyForwardedProps_DenySystemPromptReplace(t *testing.T) {
	props := map[string]any{
		"system_prompt":      "override",
		"system_prompt_mode": "replace",
	}
	req := api.Request{}
	err := applyForwardedProps(props, &req, Options{DenySystemPromptReplace: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.SystemPromptOverride.Mode != api.SystemPromptModePrepend {
		t.Errorf("mode = %q, want prepend (downgraded from replace)", req.SystemPromptOverride.Mode)
	}
}

func TestApplyForwardedProps_ToolOverride(t *testing.T) {
	props := map[string]any{
		"allowed_tools":     []any{"bash", "file_read"},
		"passthrough_tools": []any{"custom_tool"},
	}
	req := api.Request{PassthroughTools: []string{"existing"}}
	err := applyForwardedProps(props, &req, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.ToolWhitelist) != 2 || req.ToolWhitelist[0] != "bash" {
		t.Errorf("ToolWhitelist = %v, want [bash file_read]", req.ToolWhitelist)
	}
	if len(req.PassthroughTools) != 2 || req.PassthroughTools[0] != "existing" || req.PassthroughTools[1] != "custom_tool" {
		t.Errorf("PassthroughTools = %v, want [existing custom_tool]", req.PassthroughTools)
	}
}

func TestApplyForwardedProps_DenyToolOverride(t *testing.T) {
	props := map[string]any{
		"allowed_tools":     []any{"bash"},
		"passthrough_tools": []any{"custom_tool"},
	}
	req := api.Request{}
	err := applyForwardedProps(props, &req, Options{DenyToolOverride: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.ToolWhitelist != nil {
		t.Error("expected nil ToolWhitelist when DenyToolOverride=true")
	}
	if len(req.PassthroughTools) != 0 {
		t.Error("expected empty PassthroughTools when DenyToolOverride=true")
	}
}

func TestApplyForwardedProps_InvalidSystemPromptMode(t *testing.T) {
	props := map[string]any{
		"system_prompt":      "hello",
		"system_prompt_mode": "invalid",
	}
	req := api.Request{}
	err := applyForwardedProps(props, &req, Options{})
	if err == nil {
		t.Error("expected error for invalid system_prompt_mode")
	}
}
