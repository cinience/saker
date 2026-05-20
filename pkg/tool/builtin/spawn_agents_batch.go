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

var spawnAgentsBatchSchema = &tool.JSONSchema{
	Type: "object",
	Properties: map[string]interface{}{
		"items": map[string]interface{}{
			"type":        "array",
			"description": "Array of items to process. Each item has an 'id' and 'instruction' field.",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":          map[string]interface{}{"type": "string", "description": "Unique identifier for this item"},
					"instruction": map[string]interface{}{"type": "string", "description": "The task instruction for this item"},
				},
				"required": []string{"id", "instruction"},
			},
		},
		"subagent_type": map[string]interface{}{
			"type":        "string",
			"description": "Agent type for all items (e.g. general-purpose, explore)",
		},
		"model": map[string]interface{}{
			"type":        "string",
			"description": "Model tier for all items (sonnet, opus, haiku)",
			"enum":        []string{"sonnet", "opus", "haiku"},
		},
		"max_concurrency": map[string]interface{}{
			"type":        "integer",
			"description": "Maximum concurrent agents (default: 8, max: 32)",
		},
		"timeout_seconds": map[string]interface{}{
			"type":        "integer",
			"description": "Total timeout for the batch in seconds (default: 1800)",
		},
	},
	Required: []string{"items"},
}

type SpawnAgentsBatchTool struct {
	mu     sync.RWMutex
	runner AgentRunner
}

func NewSpawnAgentsBatchTool() *SpawnAgentsBatchTool { return &SpawnAgentsBatchTool{} }

func (t *SpawnAgentsBatchTool) Name() string             { return "spawn_agents_batch" }
func (t *SpawnAgentsBatchTool) Schema() *tool.JSONSchema  { return spawnAgentsBatchSchema }

func (t *SpawnAgentsBatchTool) Description() string {
	return `Spawn multiple agents in parallel to process a batch of items. Each item gets its own agent instance. Returns aggregated results when all agents complete or the timeout is reached.

Use this for embarrassingly parallel tasks: processing multiple files, analyzing multiple inputs, or running the same operation across many targets.`
}

func (t *SpawnAgentsBatchTool) SetRunner(r AgentRunner) {
	t.mu.Lock()
	t.runner = r
	t.mu.Unlock()
}

func (t *SpawnAgentsBatchTool) getRunner() AgentRunner {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.runner
}

type batchItem struct {
	ID          string
	Instruction string
}

type batchResult struct {
	ID     string `json:"id"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (t *SpawnAgentsBatchTool) Execute(ctx context.Context, params map[string]interface{}) (*tool.ToolResult, error) {
	runner := t.getRunner()
	if runner == nil {
		return nil, errors.New("spawn_agents_batch: runner not configured")
	}

	items, err := parseBatchItems(params["items"])
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errors.New("spawn_agents_batch: items array is empty")
	}

	subagentType := ""
	if v, ok := params["subagent_type"].(string); ok {
		subagentType = strings.TrimSpace(v)
	}
	model := ""
	if v, ok := params["model"].(string); ok {
		model = strings.TrimSpace(v)
	}

	maxConc := 8
	if v, ok := params["max_concurrency"].(float64); ok && v > 0 {
		maxConc = int(v)
	}
	if maxConc > 32 {
		maxConc = 32
	}

	timeout := 1800 * time.Second
	if v, ok := params["timeout_seconds"].(float64); ok && v > 0 {
		timeout = time.Duration(v) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	results := make([]batchResult, len(items))
	sem := make(chan struct{}, maxConc)
	var wg sync.WaitGroup

	for i, item := range items {
		wg.Add(1)
		go func(idx int, it batchItem) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = batchResult{ID: it.ID, Error: "timeout"}
				return
			}

			res, err := runner.SpawnAgent(ctx, SpawnAgentRequest{
				Prompt:       it.Instruction,
				SubagentType: subagentType,
				Model:        model,
			})
			if err != nil {
				results[idx] = batchResult{ID: it.ID, Error: err.Error()}
				return
			}

			waited, err := runner.WaitAgent(ctx, res.AgentID, timeout)
			if err != nil {
				results[idx] = batchResult{ID: it.ID, Error: err.Error()}
				return
			}
			if waited.TimedOut {
				results[idx] = batchResult{ID: it.ID, Error: "agent timed out"}
				return
			}
			results[idx] = batchResult{ID: it.ID, Output: waited.Output}
		}(i, item)
	}

	wg.Wait()

	completed, failed := 0, 0
	for _, r := range results {
		if r.Error != "" {
			failed++
		} else {
			completed++
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Batch complete: %d/%d succeeded, %d failed\n\n", completed, len(items), failed))
	for _, r := range results {
		if r.Error != "" {
			sb.WriteString(fmt.Sprintf("[%s] ERROR: %s\n", r.ID, r.Error))
		} else {
			output := r.Output
			if len(output) > 300 {
				output = output[:300] + "..."
			}
			sb.WriteString(fmt.Sprintf("[%s] %s\n", r.ID, output))
		}
	}

	return &tool.ToolResult{
		Success: failed == 0,
		Output:  sb.String(),
		Data: map[string]any{
			"total":     len(items),
			"completed": completed,
			"failed":    failed,
			"results":   results,
		},
	}, nil
}

func parseBatchItems(raw interface{}) ([]batchItem, error) {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, errors.New("spawn_agents_batch: items must be an array")
	}
	items := make([]batchItem, 0, len(arr))
	for i, entry := range arr {
		m, ok := entry.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("spawn_agents_batch: item[%d] must be an object", i)
		}
		id := strings.TrimSpace(fmt.Sprint(m["id"]))
		instruction := strings.TrimSpace(fmt.Sprint(m["instruction"]))
		if id == "" || instruction == "" {
			return nil, fmt.Errorf("spawn_agents_batch: item[%d] requires id and instruction", i)
		}
		items = append(items, batchItem{ID: id, Instruction: instruction})
	}
	return items, nil
}
