package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saker-ai/saker/pkg/model"
)

func TestTailRepeatCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		calls []toolCallSig
		want  int
	}{
		{"empty", nil, 0},
		{"single", []toolCallSig{{name: "a", input: "1"}}, 1},
		{"all same", []toolCallSig{
			{name: "a", input: "1"},
			{name: "a", input: "1"},
			{name: "a", input: "1"},
		}, 3},
		{"trailing repeat", []toolCallSig{
			{name: "a", input: "1"},
			{name: "b", input: "2"},
			{name: "a", input: "1"},
			{name: "a", input: "1"},
		}, 2},
		{"no repeat", []toolCallSig{
			{name: "a", input: "1"},
			{name: "b", input: "2"},
			{name: "c", input: "3"},
		}, 1},
		{"same name different input", []toolCallSig{
			{name: "a", input: "1"},
			{name: "a", input: "2"},
		}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tailRepeatCount(tc.calls)
			if got != tc.want {
				t.Errorf("tailRepeatCount = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestSameToolRepeatCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		calls []toolCallSig
		want  int
	}{
		{"empty", nil, 0},
		{"single", []toolCallSig{{name: "read", input: "a"}}, 1},
		{"same tool different inputs", []toolCallSig{
			{name: "read", input: "file-1"},
			{name: "read", input: "file-2"},
			{name: "read", input: "file-3"},
		}, 3},
		{"mixed tools trailing same", []toolCallSig{
			{name: "write", input: "x"},
			{name: "read", input: "a"},
			{name: "read", input: "b"},
		}, 2},
		{"interrupted by different tool", []toolCallSig{
			{name: "read", input: "a"},
			{name: "write", input: "x"},
			{name: "read", input: "b"},
		}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sameToolRepeatCount(tc.calls)
			if got != tc.want {
				t.Errorf("sameToolRepeatCount = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestHasPassthroughTool(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		calls       []ToolCall
		passthrough []string
		want        bool
	}{
		{"empty calls", nil, []string{"ask"}, false},
		{"empty passthrough", []ToolCall{{Name: "ask"}}, nil, false},
		{"match first", []ToolCall{{Name: "ask"}, {Name: "read"}}, []string{"ask"}, true},
		{"match second", []ToolCall{{Name: "read"}, {Name: "ask"}}, []string{"ask"}, true},
		{"no match", []ToolCall{{Name: "read"}, {Name: "write"}}, []string{"ask"}, false},
		{"multiple passthrough", []ToolCall{{Name: "confirm"}}, []string{"ask", "confirm"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasPassthroughTool(tc.calls, tc.passthrough)
			if got != tc.want {
				t.Errorf("hasPassthroughTool = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDuplicateToolCallID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		calls []ToolCall
		want  string
	}{
		{"no duplicates", []ToolCall{{ID: "a"}, {ID: "b"}}, ""},
		{"empty IDs ignored", []ToolCall{{ID: ""}, {ID: ""}}, ""},
		{"duplicate found", []ToolCall{{ID: "x"}, {ID: "y"}, {ID: "x"}}, "x"},
		{"empty slice", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := duplicateToolCallID(tc.calls)
			if got != tc.want {
				t.Errorf("duplicateToolCallID = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSanitizeOutputPathComponent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"bash_exec", "bash_exec"},
		{"read-file", "read-file"},
		{"tool with spaces!", "tool-with-spaces"},
		{"", "tool"},
		{"   ", "tool"},
		{"---", "tool"},
		{"café", "caf"},
		{"A.B.C", "A-B-C"},
		{"normal123", "normal123"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := sanitizeOutputPathComponent(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeOutputPathComponent(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestBudgetStatusMessage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ctx  *Context
		opts Options
		want string
	}{
		{"nil context", nil, Options{MaxTokens: 100}, ""},
		{"no budget", NewContext(), Options{}, ""},
		{"token budget 50%", &Context{CumulativeUsage: model.Usage{TotalTokens: 500}}, Options{MaxTokens: 1000}, "[Budget: ~50% tokens used]"},
		{"token budget over 100%", &Context{CumulativeUsage: model.Usage{TotalTokens: 1500}}, Options{MaxTokens: 1000}, "[Budget: ~100% tokens used]"},
		{"cost budget 25%", &Context{CumulativeCostUSD: 0.25}, Options{MaxBudgetUSD: 1.0}, "[Budget: ~25% cost used]"},
		{"cost budget over 100%", &Context{CumulativeCostUSD: 2.0}, Options{MaxBudgetUSD: 1.0}, "[Budget: ~100% cost used]"},
		{"token takes precedence over cost", &Context{
			CumulativeUsage:   model.Usage{TotalTokens: 300},
			CumulativeCostUSD: 0.5,
		}, Options{MaxTokens: 1000, MaxBudgetUSD: 1.0}, "[Budget: ~30% tokens used]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := budgetStatusMessage(tc.ctx, tc.opts)
			if got != tc.want {
				t.Errorf("budgetStatusMessage = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPersistAgentToolOutput(t *testing.T) {
	dir := t.TempDir()

	path, err := persistAgentToolOutput(dir, "read_file", "hello world")
	if err != nil {
		t.Fatalf("persist: %v", err)
	}
	if !strings.HasPrefix(path, filepath.Join(dir, "read_file")) {
		t.Fatalf("unexpected path: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("content mismatch: %q", data)
	}
}

func TestPersistAgentToolOutput_UnwritableDir(t *testing.T) {
	_, err := persistAgentToolOutput("/nonexistent/deep/path/that/wont/exist", "tool", "data")
	if err == nil {
		t.Fatal("expected error for unwritable dir")
	}
}

func TestPersistAgentToolOutput_EmptyBaseDir(t *testing.T) {
	path, err := persistAgentToolOutput("", "my-tool", "content")
	if err != nil {
		t.Fatalf("persist with empty base: %v", err)
	}
	defer os.Remove(path)
	if !strings.Contains(path, "agent-tool-output") {
		t.Fatalf("expected temp fallback path, got: %s", path)
	}
}

func TestAgent_PassthroughToolExits(t *testing.T) {
	mdl := &scriptedModel{
		outputs: []*ModelOutput{
			{ToolCalls: []ToolCall{
				{ID: "call-1", Name: "ask_user_question", Input: map[string]any{"q": "proceed?"}},
			}},
		},
	}
	tools := &stubTools{}
	ag, err := New(mdl, tools, Options{
		PassthroughTools: []string{"ask_user_question", "confirm_action"},
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	out, err := ag.Run(context.Background(), NewContext())
	if err != nil {
		t.Fatalf("run should not error for passthrough: %v", err)
	}
	if out.StopReason != StopReasonToolPassthrough {
		t.Fatalf("expected StopReasonToolPassthrough, got %q", out.StopReason)
	}
	if len(tools.calls) != 0 {
		t.Fatalf("passthrough tools should not be executed, got %d calls", len(tools.calls))
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].Name != "ask_user_question" {
		t.Fatalf("expected tool call to be preserved on output, got %+v", out.ToolCalls)
	}
}

func TestAgent_MultipleToolCallsAllExecuted(t *testing.T) {
	mdl := &scriptedModel{
		outputs: []*ModelOutput{
			{ToolCalls: []ToolCall{
				{ID: "c1", Name: "read", Input: map[string]any{"path": "/a"}},
				{ID: "c2", Name: "write", Input: map[string]any{"path": "/b"}},
				{ID: "c3", Name: "exec", Input: map[string]any{"cmd": "ls"}},
			}},
			{Content: "all done", Done: true},
		},
	}
	tools := &stubTools{}
	ag, err := New(mdl, tools, Options{})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	c := NewContext()
	out, err := ag.Run(context.Background(), c)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.StopReason != StopReasonCompleted {
		t.Fatalf("expected completed, got %q", out.StopReason)
	}
	if len(tools.calls) != 3 {
		t.Fatalf("expected 3 tool calls, got %d", len(tools.calls))
	}
	if len(c.ToolResults) != 3 {
		t.Fatalf("expected 3 tool results, got %d", len(c.ToolResults))
	}
	for i, name := range []string{"read", "write", "exec"} {
		if c.ToolResults[i].Name != name {
			t.Errorf("result[%d].Name = %q, want %q", i, c.ToolResults[i].Name, name)
		}
	}
}

func TestAgent_SameToolSoftThresholdWarning(t *testing.T) {
	mdl := &incrementingToolModel{name: "read"}
	tools := &stubTools{}

	var fires []int
	hook := func(_ context.Context, call ToolCall, count int) {
		if call.Name != "read" {
			t.Errorf("hook unexpected tool %q", call.Name)
		}
		fires = append(fires, count)
	}

	ag, err := New(mdl, tools, Options{
		SameToolSoftThreshold: 3,
		SameToolHardThreshold: 6,
		OnRepeatWarning:       hook,
		MaxIterations:         20,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = ag.Run(context.Background(), NewContext())
	if err == nil || !strings.Contains(err.Error(), "same tool called 6+ times") {
		t.Fatalf("expected hard threshold error, got %v", err)
	}
	if len(fires) != 1 {
		t.Fatalf("soft warning should fire once, got %d", len(fires))
	}
	if fires[0] != 3 {
		t.Fatalf("soft warning should fire at count=3, got %d", fires[0])
	}
}

func TestAgent_PrepareToolResult_TruncateWithWriteError(t *testing.T) {
	mdl := &scriptedModel{outputs: []*ModelOutput{
		{ToolCalls: []ToolCall{{Name: "big"}}},
		{Done: true},
	}}
	tools := &stubTools{out: strings.Repeat("x", 50)}
	ag, err := New(mdl, tools, Options{
		ToolResultMaxChars:  10,
		ToolResultOutputDir: "/nonexistent/path/for/test",
		MaxIterations:       3,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	c := NewContext()
	if _, err := ag.Run(context.Background(), c); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(c.ToolResults) != 1 {
		t.Fatalf("expected 1 result, got %d", len(c.ToolResults))
	}
	res := c.ToolResults[0]
	if !strings.Contains(res.Output, "Output truncated") {
		t.Fatalf("expected truncation notice, got %q", res.Output)
	}
	if !strings.Contains(res.Output, "could not be stored") {
		t.Fatalf("expected write-error notice, got %q", res.Output)
	}
	if _, ok := res.Metadata["truncate_error"]; !ok {
		t.Fatalf("expected truncate_error metadata, got %+v", res.Metadata)
	}
}

func TestAgent_BudgetStatusInjectedIntoContextValues(t *testing.T) {
	mdl := &scriptedModel{outputs: []*ModelOutput{{Done: true}}}
	ag, err := New(mdl, nil, Options{MaxTokens: 10000})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	c := NewContext()
	c.CumulativeUsage.TotalTokens = 7000
	out, err := ag.Run(context.Background(), c)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.StopReason != StopReasonCompleted {
		t.Fatalf("expected completed, got %q", out.StopReason)
	}
	msg, ok := c.Values["agent.budget_status"].(string)
	if !ok || !strings.Contains(msg, "70%") {
		t.Fatalf("expected budget status with 70%%, got %q (ok=%v)", msg, ok)
	}
}

func TestAgent_ContextCancelDuringToolExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	mdl := &scriptedModel{
		outputs: []*ModelOutput{
			{ToolCalls: []ToolCall{{ID: "c1", Name: "slow"}}},
		},
	}
	tools := &cancellingTools{cancel: cancel}
	ag, err := New(mdl, tools, Options{MaxIterations: 3})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	_, err = ag.Run(ctx, NewContext())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// cancellingTools cancels the context on first Execute call, simulating
// a user abort mid-tool.
type cancellingTools struct {
	cancel context.CancelFunc
}

func (t *cancellingTools) Execute(_ context.Context, call ToolCall, _ *Context) (ToolResult, error) {
	t.cancel()
	return ToolResult{Name: call.Name, Output: "partial"}, nil
}

