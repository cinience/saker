package agui

import (
	"fmt"
	"regexp"

	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

const maxPromptLength = 128 * 1024 // 128KB per message content

var threadIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_\-]{1,128}$`)

// validateRunInput checks that a RunAgentInput is well-formed before execution.
func validateRunInput(input aguitypes.RunAgentInput) error {
	if input.ThreadID != "" && !threadIDPattern.MatchString(input.ThreadID) {
		return fmt.Errorf("invalid thread_id format: must be 1-128 alphanumeric/dash/underscore characters")
	}
	if input.RunID != "" && !threadIDPattern.MatchString(input.RunID) {
		return fmt.Errorf("invalid run_id format: must be 1-128 alphanumeric/dash/underscore characters")
	}
	for i, msg := range input.Messages {
		if !isValidRole(msg.Role) {
			return fmt.Errorf("message[%d]: invalid role %q", i, msg.Role)
		}
		if contentLen(msg) > maxPromptLength {
			return fmt.Errorf("message[%d]: content exceeds maximum length (%d bytes)", i, maxPromptLength)
		}
	}
	return nil
}

func isValidRole(role aguitypes.Role) bool {
	switch role {
	case aguitypes.RoleUser, aguitypes.RoleAssistant, aguitypes.RoleSystem,
		aguitypes.RoleTool, aguitypes.RoleDeveloper, aguitypes.RoleActivity,
		aguitypes.RoleReasoning:
		return true
	}
	return false
}

func contentLen(msg aguitypes.Message) int {
	switch v := msg.Content.(type) {
	case string:
		return len(v)
	case []any:
		total := 0
		for _, part := range v {
			if m, ok := part.(map[string]any); ok {
				if text, ok := m["text"].(string); ok {
					total += len(text)
				}
			}
		}
		return total
	}
	return 0
}
