package model

import "strings"

// KnowledgeCutoff returns the training data cutoff date for a known model.
// Returns empty string for unknown models.
func KnowledgeCutoff(modelName string) string {
	name := strings.ToLower(modelName)
	switch {
	case strings.Contains(name, "claude-opus-4") || strings.Contains(name, "claude-sonnet-4-6"):
		return "May 2025"
	case strings.Contains(name, "claude-sonnet-4-5") || strings.Contains(name, "claude-sonnet-4-20"):
		return "March 2025"
	case strings.Contains(name, "claude-haiku-4"):
		return "February 2025"
	case strings.Contains(name, "claude-3"):
		return "August 2024"
	case strings.Contains(name, "gpt-4o") || strings.Contains(name, "gpt-4-turbo"):
		return "October 2023"
	case strings.Contains(name, "deepseek-v3") || strings.Contains(name, "deepseek-r1"):
		return "December 2024"
	case strings.Contains(name, "qwen-max") || strings.Contains(name, "qwen-plus"):
		return "2024"
	default:
		return ""
	}
}
