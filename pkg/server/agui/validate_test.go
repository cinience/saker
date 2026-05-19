package agui

import (
	"strings"
	"testing"

	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

func TestValidateRunInput_ValidRoles(t *testing.T) {
	t.Parallel()
	input := aguitypes.RunAgentInput{
		Messages: []aguitypes.Message{
			{Role: aguitypes.RoleUser, Content: "hi"},
			{Role: aguitypes.RoleAssistant, Content: "hello"},
			{Role: aguitypes.RoleSystem, Content: "system prompt"},
			{Role: aguitypes.RoleTool, Content: "result"},
			{Role: aguitypes.RoleDeveloper, Content: "dev"},
			{Role: aguitypes.RoleActivity, Content: "activity"},
			{Role: aguitypes.RoleReasoning, Content: "thinking"},
		},
	}
	if err := validateRunInput(input); err != nil {
		t.Fatalf("valid roles should pass: %v", err)
	}
}

func TestValidateRunInput_InvalidRole(t *testing.T) {
	t.Parallel()
	input := aguitypes.RunAgentInput{
		Messages: []aguitypes.Message{
			{Role: "invalid_role", Content: "hi"},
		},
	}
	err := validateRunInput(input)
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
	if !strings.Contains(err.Error(), "invalid role") {
		t.Errorf("error should mention invalid role, got: %v", err)
	}
}

func TestValidateRunInput_ThreadIDFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid alphanumeric", "thread_abc123", false},
		{"valid with dashes", "my-thread-1", false},
		{"valid short", "a", false},
		{"empty (auto-generated)", "", false},
		{"has spaces", "thread id", true},
		{"has special chars", "thread@id!", true},
		{"too long", strings.Repeat("a", 129), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := aguitypes.RunAgentInput{
				ThreadID: tt.id,
				Messages: []aguitypes.Message{{Role: aguitypes.RoleUser, Content: "hi"}},
			}
			err := validateRunInput(input)
			if (err != nil) != tt.wantErr {
				t.Errorf("threadID=%q: err=%v, wantErr=%v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestValidateRunInput_RunIDFormat(t *testing.T) {
	t.Parallel()
	input := aguitypes.RunAgentInput{
		RunID:    "has spaces here",
		Messages: []aguitypes.Message{{Role: aguitypes.RoleUser, Content: "hi"}},
	}
	err := validateRunInput(input)
	if err == nil {
		t.Fatal("expected error for invalid run_id")
	}
	if !strings.Contains(err.Error(), "invalid run_id") {
		t.Errorf("error should mention run_id, got: %v", err)
	}
}

func TestValidateRunInput_ContentLength(t *testing.T) {
	t.Parallel()
	longContent := strings.Repeat("x", maxPromptLength+1)
	input := aguitypes.RunAgentInput{
		Messages: []aguitypes.Message{
			{Role: aguitypes.RoleUser, Content: longContent},
		},
	}
	err := validateRunInput(input)
	if err == nil {
		t.Fatal("expected error for oversized content")
	}
	if !strings.Contains(err.Error(), "exceeds maximum length") {
		t.Errorf("error should mention max length, got: %v", err)
	}
}

func TestValidateRunInput_EmptyMessages(t *testing.T) {
	t.Parallel()
	input := aguitypes.RunAgentInput{
		Messages: []aguitypes.Message{},
	}
	if err := validateRunInput(input); err != nil {
		t.Fatalf("empty messages should pass validation (handled elsewhere): %v", err)
	}
}
