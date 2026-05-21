package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/saker-ai/saker/pkg/model"
)

// LLMJudge uses an LLM to evaluate creative output on multiple dimensions.
type LLMJudge struct {
	Model      model.Model
	MaxRetries int
}

// JudgeResult is the structured result from LLM evaluation.
type JudgeResult struct {
	Dimensions map[string]float64 `json:"dimensions"`
	Overall    float64            `json:"overall"`
	Reasoning  string             `json:"reasoning"`
}

// Judge evaluates the given output against the prompt using LLM-as-judge.
// The judgePrompt should instruct the LLM to output JSON with dimension scores.
func (j *LLMJudge) Judge(ctx context.Context, judgePrompt, userPrompt, output string) (*JudgeResult, error) {
	retries := j.MaxRetries
	if retries <= 0 {
		retries = 2
	}

	prompt := strings.ReplaceAll(judgePrompt, "{{.UserPrompt}}", userPrompt)
	prompt = strings.ReplaceAll(prompt, "{{.Output}}", output)

	var lastErr error
	for i := 0; i < retries; i++ {
		resp, err := j.Model.Complete(ctx, model.Request{
			System: "You are an evaluation judge. Always respond with valid JSON only, no markdown fences.",
			Messages: []model.Message{
				{Role: "user", Content: prompt},
			},
			MaxTokens: 1024,
		})
		if err != nil {
			lastErr = err
			continue
		}

		result, err := parseJudgeResponse(resp.Message.Content)
		if err != nil {
			lastErr = err
			continue
		}
		return result, nil
	}
	return nil, fmt.Errorf("LLMJudge: failed after %d retries: %w", retries, lastErr)
}

func parseJudgeResponse(content string) (*JudgeResult, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("parse judge JSON: %w", err)
	}

	result := &JudgeResult{
		Dimensions: make(map[string]float64),
	}

	for k, v := range raw {
		switch k {
		case "overall":
			result.Overall = toFloat(v)
		case "reasoning":
			if s, ok := v.(string); ok {
				result.Reasoning = s
			}
		default:
			result.Dimensions[k] = toFloat(v)
		}
	}

	// Normalize: if scores are on 1-5 scale, convert to [0,1]
	if result.Overall > 1.0 {
		result.Overall = (result.Overall - 1.0) / 4.0
	}
	for k, v := range result.Dimensions {
		if v > 1.0 {
			result.Dimensions[k] = (v - 1.0) / 4.0
		}
	}

	return result, nil
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}
