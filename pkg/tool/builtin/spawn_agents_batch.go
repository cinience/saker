package toolbuiltin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/saker-ai/saker/pkg/tool"
)

// Compile-time check: SpawnAgentsBatchTool implements StreamingTool.
var _ tool.StreamingTool = (*SpawnAgentsBatchTool)(nil)

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
		"max_output_chars": map[string]interface{}{
			"type":        "integer",
			"description": "Max characters per item output (default: 2000)",
		},
	},
	Required: []string{"items"},
}

type SpawnAgentsBatchTool struct {
	mu     sync.RWMutex
	runner AgentRunner
}

func NewSpawnAgentsBatchTool() *SpawnAgentsBatchTool { return &SpawnAgentsBatchTool{} }

func (t *SpawnAgentsBatchTool) Name() string            { return "spawn_agents_batch" }
func (t *SpawnAgentsBatchTool) Schema() *tool.JSONSchema { return spawnAgentsBatchSchema }

func (t *SpawnAgentsBatchTool) Description() string {
	return `Spawn multiple agents in parallel to process a batch of items. Each item gets its own agent instance. Returns aggregated results with per-agent timing and metadata when all agents complete or the timeout is reached.

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
	ID      string        `json:"id"`
	Output  string        `json:"output,omitempty"`
	Error   string        `json:"error,omitempty"`
	Profile string        `json:"profile,omitempty"`
	Model   string        `json:"model,omitempty"`
	Elapsed time.Duration `json:"elapsed_ms,omitempty"`
}

// Execute implements tool.Tool (non-streaming fallback).
func (t *SpawnAgentsBatchTool) Execute(ctx context.Context, params map[string]interface{}) (*tool.ToolResult, error) {
	return t.run(ctx, params, nil)
}

// StreamExecute implements tool.StreamingTool with progress reporting.
func (t *SpawnAgentsBatchTool) StreamExecute(ctx context.Context, params map[string]interface{}, emit func(chunk string, isStderr bool)) (*tool.ToolResult, error) {
	return t.run(ctx, params, emit)
}

func (t *SpawnAgentsBatchTool) run(ctx context.Context, params map[string]interface{}, emit func(string, bool)) (*tool.ToolResult, error) {
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

	maxOutputChars := 2000
	if v, ok := params["max_output_chars"].(float64); ok && v > 0 {
		maxOutputChars = int(v)
	}

	timeout := 1800 * time.Second
	if v, ok := params["timeout_seconds"].(float64); ok && v > 0 {
		timeout = time.Duration(v) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	batchStart := time.Now()

	// Emit startup summary.
	if emit != nil {
		var sb strings.Builder
		agentType := subagentType
		if agentType == "" {
			agentType = "default"
		}
		sb.WriteString(fmt.Sprintf("[batch] Spawning %d agents (type: %s", len(items), agentType))
		if model != "" {
			sb.WriteString(fmt.Sprintf(", model: %s", model))
		}
		sb.WriteString(fmt.Sprintf(", concurrency: %d)\n", maxConc))
		for _, it := range items {
			instr := it.Instruction
			if len(instr) > 80 {
				instr = instr[:77] + "..."
			}
			sb.WriteString(fmt.Sprintf("  • [%s] %s\n", it.ID, instr))
		}
		emit(sb.String(), false)
	}

	results := make([]batchResult, len(items))
	sem := make(chan struct{}, maxConc)
	var wg sync.WaitGroup
	var completed atomic.Int32

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
				n := completed.Add(1)
				if emit != nil {
					emit(fmt.Sprintf("[progress] %d/%d done — [%s] ERROR: %s\n",
						n, len(items), it.ID, err.Error()), false)
				}
				return
			}

			waited, err := runner.WaitAgent(ctx, res.AgentID, timeout)
			if err != nil {
				results[idx] = batchResult{ID: it.ID, Error: err.Error()}
				n := completed.Add(1)
				if emit != nil {
					emit(fmt.Sprintf("[progress] %d/%d done — [%s] ERROR: %s\n",
						n, len(items), it.ID, err.Error()), false)
				}
				return
			}
			if waited.TimedOut {
				results[idx] = batchResult{ID: it.ID, Error: "agent timed out", Profile: waited.Profile, Model: waited.Model, Elapsed: waited.Elapsed}
				n := completed.Add(1)
				if emit != nil {
					emit(fmt.Sprintf("[progress] %d/%d done — [%s] TIMEOUT (%s)\n",
						n, len(items), it.ID, formatDuration(waited.Elapsed)), false)
				}
				return
			}

			results[idx] = batchResult{
				ID:      it.ID,
				Output:  waited.Output,
				Profile: waited.Profile,
				Model:   waited.Model,
				Elapsed: waited.Elapsed,
			}
			n := completed.Add(1)
			if emit != nil {
				emit(fmt.Sprintf("[progress] %d/%d done — [%s] completed (%s)\n",
					n, len(items), it.ID, formatDuration(waited.Elapsed)), false)
			}
		}(i, item)
	}

	wg.Wait()
	totalElapsed := time.Since(batchStart)

	succeeded, failed := 0, 0
	for _, r := range results {
		if r.Error != "" {
			failed++
		} else {
			succeeded++
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Batch complete: %d/%d succeeded, %d failed (total: %s)\n\n",
		succeeded, len(items), failed, formatDuration(totalElapsed)))

	for _, r := range results {
		meta := formatMeta(r.Profile, r.Elapsed)
		if r.Error != "" {
			sb.WriteString(fmt.Sprintf("[%s] %sERROR: %s\n", r.ID, meta, r.Error))
		} else {
			output := r.Output
			if len(output) > maxOutputChars {
				output = output[:maxOutputChars] + "..."
			}
			sb.WriteString(fmt.Sprintf("[%s] %s%s\n", r.ID, meta, output))
		}
	}

	return &tool.ToolResult{
		Success: failed == 0,
		Output:  sb.String(),
		Data: map[string]any{
			"total":         len(items),
			"completed":     succeeded,
			"failed":        failed,
			"total_elapsed": totalElapsed.Milliseconds(),
			"results":       results,
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

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "—"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func formatMeta(profile string, elapsed time.Duration) string {
	if profile == "" && elapsed == 0 {
		return ""
	}
	parts := make([]string, 0, 2)
	if profile != "" {
		parts = append(parts, profile)
	}
	if elapsed > 0 {
		parts = append(parts, formatDuration(elapsed))
	}
	return "(" + strings.Join(parts, ", ") + ") "
}
