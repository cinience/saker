package toolbuiltin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/saker-ai/saker/pkg/tool"
)

var spawnAgentSchema = &tool.JSONSchema{
	Type: "object",
	Properties: map[string]interface{}{
		"prompt": map[string]interface{}{
			"type":        "string",
			"description": "The task for the agent to perform",
		},
		"subagent_type": map[string]interface{}{
			"type":        "string",
			"description": "The type of specialized agent to use (e.g. general-purpose, explore, plan)",
		},
		"model": map[string]interface{}{
			"type":        "string",
			"description": "Optional model tier override (sonnet, opus, haiku)",
			"enum":        []string{"sonnet", "opus", "haiku"},
		},
		"fork_context": map[string]interface{}{
			"type":        "boolean",
			"description": "When true, the agent inherits the parent's conversation history for context sharing",
		},
		"fork_turns": map[string]interface{}{
			"type":        "integer",
			"description": "Number of recent conversation turns to copy when fork_context is true (0 = all)",
		},
		"run_in_background": map[string]interface{}{
			"type":        "boolean",
			"description": "When true, spawns the agent in the background and returns immediately with an ID",
		},
	},
	Required: []string{"prompt"},
}

type SpawnAgentTool struct {
	mu     sync.RWMutex
	runner AgentRunner
}

func NewSpawnAgentTool() *SpawnAgentTool { return &SpawnAgentTool{} }

func (t *SpawnAgentTool) Name() string        { return "spawn_agent" }
func (t *SpawnAgentTool) Schema() *tool.JSONSchema { return spawnAgentSchema }

func (t *SpawnAgentTool) Description() string {
	return `Launch a new agent to handle a task. Returns the agent's ID and nickname for later reference via send_input, wait_agent, or close_agent.

Use spawn_agent for complex multi-step tasks that benefit from an independent execution context. Prefer spawning multiple agents in parallel (multiple tool calls in one message) when tasks are independent.`
}

func (t *SpawnAgentTool) SetRunner(r AgentRunner) {
	t.mu.Lock()
	t.runner = r
	t.mu.Unlock()
}

func (t *SpawnAgentTool) getRunner() AgentRunner {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.runner
}

func (t *SpawnAgentTool) Execute(ctx context.Context, params map[string]interface{}) (*tool.ToolResult, error) {
	runner := t.getRunner()
	if runner == nil {
		return nil, errors.New("spawn_agent: runner not configured")
	}

	prompt := strings.TrimSpace(fmt.Sprint(params["prompt"]))
	if prompt == "" {
		return nil, errors.New("spawn_agent: prompt is required")
	}

	req := SpawnAgentRequest{Prompt: prompt}
	if v, ok := params["subagent_type"].(string); ok {
		req.SubagentType = strings.TrimSpace(v)
	}
	if v, ok := params["model"].(string); ok {
		req.Model = strings.TrimSpace(v)
	}
	if v, ok := params["fork_context"].(bool); ok {
		req.ForkContext = v
	}
	if v, ok := params["fork_turns"].(float64); ok {
		req.ForkTurns = int(v)
	}
	if v, ok := params["run_in_background"].(bool); ok {
		req.Background = v
	}

	result, err := runner.SpawnAgent(ctx, req)
	if err != nil {
		return nil, err
	}

	output := fmt.Sprintf("Agent spawned: %s (nickname: %s)", result.AgentID, result.Nickname)
	if req.Background {
		output = fmt.Sprintf("Agent launched in background: %s (nickname: %s)", result.AgentID, result.Nickname)
	}

	return &tool.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]any{
			"agent_id": result.AgentID,
			"nickname": result.Nickname,
		},
	}, nil
}
