package eval

import (
	"testing"
)

func TestParseJudgeResponse_Scale1to5(t *testing.T) {
	input := `{"structure": 4, "creativity": 3.5, "constraint": 5, "professionalism": 4, "overall": 4}`
	result, err := parseJudgeResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// overall=4 => (4-1)/4 = 0.75
	if result.Overall < 0.74 || result.Overall > 0.76 {
		t.Errorf("overall: want ~0.75, got %f", result.Overall)
	}
	// structure=4 => 0.75
	if v := result.Dimensions["structure"]; v < 0.74 || v > 0.76 {
		t.Errorf("structure: want ~0.75, got %f", v)
	}
	// creativity=3.5 => (3.5-1)/4 = 0.625
	if v := result.Dimensions["creativity"]; v < 0.62 || v > 0.63 {
		t.Errorf("creativity: want ~0.625, got %f", v)
	}
	// constraint=5 => (5-1)/4 = 1.0
	if v := result.Dimensions["constraint"]; v < 0.99 || v > 1.01 {
		t.Errorf("constraint: want 1.0, got %f", v)
	}
}

func TestParseJudgeResponse_Scale0to1(t *testing.T) {
	input := `{"specificity": 0.8, "feasibility": 0.9, "overall": 0.85}`
	result, err := parseJudgeResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Overall != 0.85 {
		t.Errorf("overall: want 0.85, got %f", result.Overall)
	}
	if result.Dimensions["specificity"] != 0.8 {
		t.Errorf("specificity: want 0.8, got %f", result.Dimensions["specificity"])
	}
}

func TestParseJudgeResponse_WithMarkdownFences(t *testing.T) {
	input := "```json\n{\"overall\": 4, \"creativity\": 3}\n```"
	result, err := parseJudgeResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Overall < 0.74 || result.Overall > 0.76 {
		t.Errorf("overall: want ~0.75, got %f", result.Overall)
	}
}

func TestParseJudgeResponse_WithReasoning(t *testing.T) {
	input := `{"overall": 4, "reasoning": "Good structure but lacks originality"}`
	result, err := parseJudgeResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reasoning != "Good structure but lacks originality" {
		t.Errorf("reasoning: got %q", result.Reasoning)
	}
}

func TestParseJudgeResponse_InvalidJSON(t *testing.T) {
	_, err := parseJudgeResponse("not json at all")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMedian(t *testing.T) {
	tests := []struct {
		input []float64
		want  float64
	}{
		{[]float64{1, 2, 3}, 2.0},
		{[]float64{1, 2, 3, 4}, 2.5},
		{[]float64{5}, 5.0},
		{[]float64{}, 0.0},
		{[]float64{0.3, 0.7, 0.9}, 0.7},
	}
	for _, tt := range tests {
		got := median(tt.input)
		if got != tt.want {
			t.Errorf("median(%v): want %f, got %f", tt.input, tt.want, got)
		}
	}
}

func TestNormalize1to5(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{1.0, 0.0},
		{3.0, 0.5},
		{5.0, 1.0},
		{0.5, 0.0},  // clamped to min 1
		{6.0, 1.0},  // clamped to max 5
	}
	for _, tt := range tests {
		got := normalize1to5(tt.input)
		if got != tt.want {
			t.Errorf("normalize1to5(%f): want %f, got %f", tt.input, tt.want, got)
		}
	}
}

func TestDetectScale(t *testing.T) {
	tests := []struct {
		name string
		r    *JudgeResult
		want scoreScale
	}{
		{
			name: "0to1_scale",
			r:    &JudgeResult{Overall: 0.8, Dimensions: map[string]float64{"a": 0.9}},
			want: scale0to1,
		},
		{
			name: "1to5_overall",
			r:    &JudgeResult{Overall: 4.0, Dimensions: map[string]float64{"a": 0.9}},
			want: scale1to5,
		},
		{
			name: "1to5_dimension",
			r:    &JudgeResult{Overall: 0.8, Dimensions: map[string]float64{"a": 3.5}},
			want: scale1to5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectScale(tt.r)
			if got != tt.want {
				t.Errorf("detectScale: want %v, got %v", tt.want, got)
			}
		})
	}
}
