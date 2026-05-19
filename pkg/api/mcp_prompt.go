package api

import (
	"fmt"
	"strings"
)

// MCPServerInfo holds the name and instructions for an MCP server.
type MCPServerInfo struct {
	Name         string
	Instructions string
}

// BuildMCPInstructionsSection formats MCP server instructions for the system prompt.
// Returns empty string if no servers have instructions.
func BuildMCPInstructionsSection(servers []MCPServerInfo) string {
	var withInstructions []MCPServerInfo
	for _, s := range servers {
		if strings.TrimSpace(s.Instructions) != "" {
			withInstructions = append(withInstructions, s)
		}
	}

	if len(withInstructions) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# MCP Server Instructions\n\n")
	sb.WriteString("The following MCP servers have provided instructions for how to use their tools and resources:\n\n")

	for _, s := range withInstructions {
		sb.WriteString(fmt.Sprintf("## %s\n%s\n\n", s.Name, s.Instructions))
	}

	return strings.TrimRight(sb.String(), "\n")
}
