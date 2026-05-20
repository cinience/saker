package toolbuiltin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/saker-ai/saker/pkg/tool"
)

var sendInputSchema = &tool.JSONSchema{
	Type: "object",
	Properties: map[string]interface{}{
		"agent_id": map[string]interface{}{
			"type":        "string",
			"description": "The agent ID or nickname to send the message to",
		},
		"message": map[string]interface{}{
			"type":        "string",
			"description": "The message content to deliver to the running agent",
		},
	},
	Required: []string{"agent_id", "message"},
}

type SendInputTool struct {
	mu     sync.RWMutex
	runner AgentRunner
}

func NewSendInputTool() *SendInputTool { return &SendInputTool{} }

func (t *SendInputTool) Name() string             { return "send_input" }
func (t *SendInputTool) Schema() *tool.JSONSchema  { return sendInputSchema }

func (t *SendInputTool) Description() string {
	return `Send a message to a running agent. The message is injected into the agent's context and processed on its next iteration. Use this for providing additional instructions, corrections, or context to a running agent.`
}

func (t *SendInputTool) SetRunner(r AgentRunner) {
	t.mu.Lock()
	t.runner = r
	t.mu.Unlock()
}

func (t *SendInputTool) getRunner() AgentRunner {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.runner
}

func (t *SendInputTool) Execute(ctx context.Context, params map[string]interface{}) (*tool.ToolResult, error) {
	runner := t.getRunner()
	if runner == nil {
		return nil, errors.New("send_input: runner not configured")
	}

	agentID := strings.TrimSpace(fmt.Sprint(params["agent_id"]))
	if agentID == "" {
		return nil, errors.New("send_input: agent_id is required")
	}
	msg := strings.TrimSpace(fmt.Sprint(params["message"]))
	if msg == "" {
		return nil, errors.New("send_input: message is required")
	}

	if err := runner.SendInput(ctx, agentID, msg); err != nil {
		return &tool.ToolResult{
			Success: false,
			Output:  fmt.Sprintf("Failed to send message: %v", err),
		}, nil
	}

	return &tool.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Message delivered to agent %s", agentID),
	}, nil
}
