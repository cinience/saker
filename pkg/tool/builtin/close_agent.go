package toolbuiltin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/saker-ai/saker/pkg/tool"
)

var closeAgentSchema = &tool.JSONSchema{
	Type: "object",
	Properties: map[string]interface{}{
		"agent_id": map[string]interface{}{
			"type":        "string",
			"description": "The agent ID or nickname to close/cancel",
		},
	},
	Required: []string{"agent_id"},
}

type CloseAgentTool struct {
	mu     sync.RWMutex
	runner AgentRunner
}

func NewCloseAgentTool() *CloseAgentTool { return &CloseAgentTool{} }

func (t *CloseAgentTool) Name() string             { return "close_agent" }
func (t *CloseAgentTool) Schema() *tool.JSONSchema  { return closeAgentSchema }

func (t *CloseAgentTool) Description() string {
	return `Cancel a running agent and retrieve any partial output. Use when an agent is no longer needed or has exceeded its useful lifetime.`
}

func (t *CloseAgentTool) SetRunner(r AgentRunner) {
	t.mu.Lock()
	t.runner = r
	t.mu.Unlock()
}

func (t *CloseAgentTool) getRunner() AgentRunner {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.runner
}

func (t *CloseAgentTool) Execute(ctx context.Context, params map[string]interface{}) (*tool.ToolResult, error) {
	runner := t.getRunner()
	if runner == nil {
		return nil, errors.New("close_agent: runner not configured")
	}

	agentID := strings.TrimSpace(fmt.Sprint(params["agent_id"]))
	if agentID == "" {
		return nil, errors.New("close_agent: agent_id is required")
	}

	result, err := runner.CloseAgent(ctx, agentID)
	if err != nil {
		return &tool.ToolResult{
			Success: false,
			Output:  fmt.Sprintf("Error closing agent: %v", err),
		}, nil
	}

	output := fmt.Sprintf("Agent %s closed (status: %s)", result.AgentID, result.Status)
	if result.Output != "" {
		output += "\n\n" + result.Output
	}

	return &tool.ToolResult{
		Success: true,
		Output:  output,
		Data:    map[string]any{"agent_id": result.AgentID, "status": result.Status},
	}, nil
}
