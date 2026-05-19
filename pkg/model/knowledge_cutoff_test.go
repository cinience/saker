package model

import "testing"

func TestKnowledgeCutoff(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{"claude-opus-4-20250514", "May 2025"},
		{"claude-sonnet-4-6-20250514", "May 2025"},
		{"claude-sonnet-4-5-20250409", "March 2025"},
		{"claude-sonnet-4-20250514", "March 2025"},
		{"claude-haiku-4-5-20250401", "February 2025"},
		{"claude-3-5-sonnet-20241022", "August 2024"},
		{"gpt-4o-2024-08-06", "October 2023"},
		{"gpt-4-turbo-2024-04-09", "October 2023"},
		{"deepseek-v3", "December 2024"},
		{"deepseek-r1", "December 2024"},
		{"qwen-max-2025-01", "2024"},
		{"qwen-plus-2025-04", "2024"},
		{"unknown-model-x", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := KnowledgeCutoff(tt.model)
			if got != tt.want {
				t.Errorf("KnowledgeCutoff(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}
