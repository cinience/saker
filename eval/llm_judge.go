package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/saker-ai/saker/pkg/model"
)

// LLMJudge uses an LLM to evaluate creative output on multiple dimensions.
type LLMJudge struct {
	Model      model.Model
	MaxRetries int
	Runs       int // Number of evaluation runs; takes median. Default 1.
}

// JudgeResult is the structured result from LLM evaluation.
type JudgeResult struct {
	Dimensions map[string]float64 `json:"dimensions"`
	Overall    float64            `json:"overall"`
	Reasoning  string             `json:"reasoning"`
}

// Judge evaluates the given output against the prompt using LLM-as-judge.
// When Runs > 1, it runs multiple evaluations and takes the median score.
func (j *LLMJudge) Judge(ctx context.Context, judgePrompt, userPrompt, output string) (*JudgeResult, error) {
	runs := j.Runs
	if runs <= 0 {
		runs = 1
	}

	var results []*JudgeResult
	var lastErr error

	for r := 0; r < runs; r++ {
		result, err := j.singleJudge(ctx, judgePrompt, userPrompt, output)
		if err != nil {
			lastErr = err
			continue
		}
		results = append(results, result)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("LLMJudge: all %d runs failed: %w", runs, lastErr)
	}

	if len(results) == 1 {
		return results[0], nil
	}

	return medianResult(results), nil
}

func (j *LLMJudge) singleJudge(ctx context.Context, judgePrompt, userPrompt, output string) (*JudgeResult, error) {
	retries := j.MaxRetries
	if retries <= 0 {
		retries = 2
	}

	prompt := strings.ReplaceAll(judgePrompt, "{{.UserPrompt}}", userPrompt)
	prompt = strings.ReplaceAll(prompt, "{{.Output}}", output)

	var lastErr error
	for i := 0; i < retries; i++ {
		resp, err := j.Model.Complete(ctx, model.Request{
			System: "You are an evaluation judge. Always respond with valid JSON only, no markdown fences. Score each dimension on a 1-5 scale.",
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
	return nil, fmt.Errorf("singleJudge: failed after %d retries: %w", retries, lastErr)
}

func medianResult(results []*JudgeResult) *JudgeResult {
	overalls := make([]float64, len(results))
	for i, r := range results {
		overalls[i] = r.Overall
	}
	sort.Float64s(overalls)

	// Collect all dimension keys
	dimKeys := map[string]bool{}
	for _, r := range results {
		for k := range r.Dimensions {
			dimKeys[k] = true
		}
	}

	medianDims := make(map[string]float64, len(dimKeys))
	for k := range dimKeys {
		vals := make([]float64, 0, len(results))
		for _, r := range results {
			if v, ok := r.Dimensions[k]; ok {
				vals = append(vals, v)
			}
		}
		sort.Float64s(vals)
		medianDims[k] = median(vals)
	}

	return &JudgeResult{
		Dimensions: medianDims,
		Overall:    median(overalls),
		Reasoning:  results[len(results)/2].Reasoning,
	}
}

func median(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2.0
	}
	return sorted[n/2]
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

	// Detect scale: if overall or any dimension > 1.0, assume 1-5 scale
	scale := detectScale(result)
	if scale == scale1to5 {
		result.Overall = normalize1to5(result.Overall)
		for k, v := range result.Dimensions {
			result.Dimensions[k] = normalize1to5(v)
		}
	}

	return result, nil
}

type scoreScale int

const (
	scale0to1 scoreScale = iota
	scale1to5
)

func detectScale(r *JudgeResult) scoreScale {
	if r.Overall > 1.0 {
		return scale1to5
	}
	for _, v := range r.Dimensions {
		if v > 1.0 {
			return scale1to5
		}
	}
	return scale0to1
}

func normalize1to5(v float64) float64 {
	if v < 1.0 {
		v = 1.0
	}
	if v > 5.0 {
		v = 5.0
	}
	return (v - 1.0) / 4.0
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
