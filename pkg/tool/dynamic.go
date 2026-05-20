package tool

import "github.com/saker-ai/saker/pkg/model"

// DynamicToolSource is an optional per-request tool provider for dynamically
// registered tools (e.g., MCP servers injected by AG-UI clients). The runtime
// falls back to this source when the primary registry does not contain a tool.
type DynamicToolSource interface {
	LookupTool(name string) (Tool, bool)
	ListToolDefs() []model.ToolDefinition
}

// DynamicInstructionSource is an optional interface that a DynamicToolSource
// can implement to provide MCP server instructions for injection into the
// system prompt. The runtime checks for this via type assertion.
type DynamicInstructionSource interface {
	MCPInstructions() map[string]string
}
