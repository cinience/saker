//go:build integration

package creative_tool_selection_eval

import (
	"context"
	"strings"
	"testing"

	"github.com/saker-ai/saker/eval"
	"github.com/saker-ai/saker/pkg/model"
	"github.com/saker-ai/saker/pkg/testutil"
)

type toolSelectionCase struct {
	Name           string
	Prompt         string
	ExpectedTool   string
	AcceptedTools  []string
	ExpectedParams map[string]string
	IsChain        bool
	ChainTools     []string
}

func TestEval_CreativeToolSelection(t *testing.T) {
	testutil.RequireIntegration(t)

	suite := &eval.EvalSuite{Name: "creative_tool_selection"}
	mdl := eval.NewLLMModel(t, "")

	for _, tc := range creativeToolCases() {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			resp, err := mdl.Complete(context.Background(), model.Request{
				System: `You are a creative production assistant with access to specialized media and canvas tools. When the user asks you to perform a task, select the most appropriate tool(s). Always use tool calls, never respond with plain text. For multi-step tasks, call multiple tools in the correct order.`,
				Messages: []model.Message{
					{Role: "user", Content: tc.Prompt},
				},
				Tools:     creativeToolDefinitions(),
				MaxTokens: 2048,
			})
			if err != nil {
				t.Fatalf("Complete: %v", err)
			}

			score := evaluateToolSelection(tc, resp)
			pass := score >= 0.5

			suite.Add(eval.EvalResult{
				Name:     tc.Name,
				Pass:     pass,
				Score:    score,
				Expected: tc.ExpectedTool,
				Got:      extractToolNames(resp),
				Details: map[string]any{
					"is_chain": tc.IsChain,
					"score":    score,
				},
			})

			if !pass {
				t.Logf("case %q: want %s, got %s (score=%.2f)",
					tc.Name, tc.ExpectedTool, extractToolNames(resp), score)
			}
		})
	}

	t.Cleanup(func() {
		t.Logf("\n%s", suite.Summary())
		if suite.PassRate() < 0.75 {
			t.Errorf("creative_tool_selection pass rate %.1f%% below 75%% threshold", suite.PassRate()*100)
		}
	})
}

func evaluateToolSelection(tc toolSelectionCase, resp *model.Response) float64 {
	if len(resp.Message.ToolCalls) == 0 {
		return 0.0
	}

	score := 0.0

	if tc.IsChain {
		// Multi-tool chain evaluation
		gotTools := make([]string, len(resp.Message.ToolCalls))
		for i, call := range resp.Message.ToolCalls {
			gotTools[i] = call.Name
		}

		// Tool match: check if expected tools are present
		matchCount := 0
		for _, expected := range tc.ChainTools {
			for _, got := range gotTools {
				if got == expected {
					matchCount++
					break
				}
			}
		}
		if len(tc.ChainTools) > 0 {
			score += 0.5 * float64(matchCount) / float64(len(tc.ChainTools))
		}

		// Order match: check if tools are in correct order
		orderCorrect := true
		lastIdx := -1
		for _, expected := range tc.ChainTools {
			found := false
			for i, got := range gotTools {
				if got == expected && i > lastIdx {
					lastIdx = i
					found = true
					break
				}
			}
			if !found {
				orderCorrect = false
				break
			}
		}
		if orderCorrect {
			score += 0.2
		}

		// Param match (check first tool)
		if len(tc.ExpectedParams) > 0 && len(resp.Message.ToolCalls) > 0 {
			paramScore := evaluateParams(tc.ExpectedParams, resp.Message.ToolCalls[0].Arguments)
			score += 0.3 * paramScore
		} else {
			score += 0.3
		}
	} else {
		// Single tool evaluation
		call := resp.Message.ToolCalls[0]
		toolMatch := call.Name == tc.ExpectedTool
		if !toolMatch {
			for _, alt := range tc.AcceptedTools {
				if call.Name == alt {
					toolMatch = true
					break
				}
			}
		}

		if toolMatch {
			score += 0.5
		}

		// Param match
		if len(tc.ExpectedParams) > 0 {
			paramScore := evaluateParams(tc.ExpectedParams, call.Arguments)
			score += 0.5 * paramScore
		} else if toolMatch {
			score += 0.5
		}
	}

	return score
}

func evaluateParams(expected map[string]string, got map[string]any) float64 {
	if len(expected) == 0 {
		return 1.0
	}
	matched := 0
	for key, substr := range expected {
		val, ok := got[key]
		if !ok {
			continue
		}
		valStr, _ := val.(string)
		if strings.Contains(strings.ToLower(valStr), strings.ToLower(substr)) {
			matched++
		}
	}
	return float64(matched) / float64(len(expected))
}

func extractToolNames(resp *model.Response) string {
	if len(resp.Message.ToolCalls) == 0 {
		return "(no tool call)"
	}
	names := make([]string, len(resp.Message.ToolCalls))
	for i, tc := range resp.Message.ToolCalls {
		names[i] = tc.Name
	}
	return strings.Join(names, " -> ")
}
