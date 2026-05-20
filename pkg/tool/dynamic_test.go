package tool_test

import (
	"testing"

	"github.com/saker-ai/saker/pkg/model"
	"github.com/saker-ai/saker/pkg/tool"
)

// Verify sessionMCPRegistry satisfies DynamicToolSource at compile time.
// The actual implementation lives in pkg/server/agui, so here we just verify
// the interface is usable.
func TestDynamicToolSourceInterface(t *testing.T) {
	var _ tool.DynamicToolSource = (*mockDynamicSource)(nil)
}

type mockDynamicSource struct{}

func (m *mockDynamicSource) LookupTool(name string) (tool.Tool, bool) { return nil, false }
func (m *mockDynamicSource) ListToolDefs() []model.ToolDefinition      { return nil }
