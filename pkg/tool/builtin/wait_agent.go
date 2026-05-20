package toolbuiltin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/saker-ai/saker/pkg/tool"
)

var waitAgentSchema = &tool.JSONSchema{
	Type: "object",
	Properties: map[string]interface{}{
		"agent_id": map[string]interface{}{
			"type":        "string",
			"description": "The agent ID or nickname to wait for",
		},
		"timeout_ms": map[string]interface{}{
			"type":        "integer",
			"description": "Maximum time to wait in milliseconds (default: 600000 = 10 minutes)",
		},
	},
	Required: []string{"agent_id"},
}

type WaitAgentTool struct {
	mu     sync.RWMutex
	runner AgentRunner
}

func NewWaitAgentTool() *WaitAgentTool { return &WaitAgentTool{} }

func (t *WaitAgentTool) Name() string             { return "wait_agent" }
func (t *WaitAgentTool) Schema() *tool.JSONSchema  { return waitAgentSchema }

func (t *WaitAgentTool) Description() string {
	return `Wait for a running agent to complete and return its output. Blocks until the agent finishes or the timeout is reached.`
}

func (t *WaitAgentTool) SetRunner(r AgentRunner) {
	t.mu.Lock()
	t.runner = r
	t.mu.Unlock()
}

func (t *WaitAgentTool) getRunner() AgentRunner {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.runner
}

func (t *WaitAgentTool) Execute(ctx context.Context, params map[string]interface{}) (*tool.ToolResult, error) {
	runner := t.getRunner()
	if runner == nil {
		return nil, errors.New("wait_agent: runner not configured")
	}

	agentID := strings.TrimSpace(fmt.Sprint(params["agent_id"]))
	if agentID == "" {
		return nil, errors.New("wait_agent: agent_id is required")
	}

	timeout := 10 * time.Minute
	if v, ok := params["timeout_ms"].(float64); ok && v > 0 {
		timeout = time.Duration(v) * time.Millisecond
	}

	result, err := runner.WaitAgent(ctx, agentID, timeout)
	if err != nil {
		return &tool.ToolResult{
			Success: false,
			Output:  fmt.Sprintf("Error waiting for agent: %v", err),
		}, nil
	}

	if result.TimedOut {
		return &tool.ToolResult{
			Success: false,
			Output:  fmt.Sprintf("Agent %s timed out (still running)", agentID),
			Data:    map[string]any{"agent_id": result.AgentID, "status": "timeout"},
		}, nil
	}

	return &tool.ToolResult{
		Success: result.Status == "completed",
		Output:  result.Output,
		Data:    map[string]any{"agent_id": result.AgentID, "status": result.Status},
	}, nil
}
