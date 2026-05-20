package agui

import (
	"context"
	"log/slog"
	"testing"

	"github.com/saker-ai/saker/pkg/tool"
)

// Compile-time interface satisfaction checks.
var (
	_ tool.DynamicToolSource        = (*sessionMCPRegistry)(nil)
	_ tool.DynamicInstructionSource = (*sessionMCPRegistry)(nil)
)

func TestSessionMCPRegistry_CloseIdempotent(t *testing.T) {
	reg := newSessionMCPRegistry(slog.Default())
	reg.Close()
	reg.Close() // should not panic
}

func TestSessionMCPRegistry_LookupAfterClose(t *testing.T) {
	reg := newSessionMCPRegistry(slog.Default())
	reg.Close()

	_, ok := reg.LookupTool("anything")
	if ok {
		t.Fatal("expected LookupTool to return false after close")
	}
}

func TestSessionMCPRegistry_ListToolDefsAfterClose(t *testing.T) {
	reg := newSessionMCPRegistry(slog.Default())
	reg.Close()

	defs := reg.ListToolDefs()
	if defs != nil {
		t.Fatalf("expected nil defs after close, got %v", defs)
	}
}

func TestSessionMCPRegistry_EnsureServersAfterClose(t *testing.T) {
	reg := newSessionMCPRegistry(slog.Default())
	reg.Close()

	err := reg.EnsureServers(context.Background(), []ClientMCPServer{
		{Name: "x", Type: "http", URL: "https://example.com"},
	}, nil)
	if err == nil {
		t.Fatal("expected error calling EnsureServers on closed registry")
	}
}

func TestSessionMCPRegistry_ServerNames(t *testing.T) {
	reg := newSessionMCPRegistry(slog.Default())

	// Manually inject entries to test ServerNames without real connections.
	reg.entries["beta"] = &mcpEntry{
		server:   ClientMCPServer{Name: "beta"},
		registry: tool.NewRegistry(),
	}
	reg.entries["alpha"] = &mcpEntry{
		server:   ClientMCPServer{Name: "alpha"},
		registry: tool.NewRegistry(),
	}

	names := reg.ServerNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("expected [alpha, beta], got %v", names)
	}

	reg.Close()
}

func TestSessionMCPRegistry_IncrementalDiff_NoChange(t *testing.T) {
	reg := newSessionMCPRegistry(slog.Default())

	// Pre-populate an entry.
	srv := ClientMCPServer{Name: "srv1", Type: "http", URL: "https://example.com/mcp"}
	reg.entries["srv1"] = &mcpEntry{
		server:   srv,
		registry: tool.NewRegistry(),
	}

	// EnsureServers with the same server should be a no-op (no connect attempt).
	err := reg.EnsureServers(context.Background(), []ClientMCPServer{srv}, nil)
	if err != nil {
		t.Fatalf("unexpected error on no-change: %v", err)
	}
	// Entry should still exist.
	if _, ok := reg.entries["srv1"]; !ok {
		t.Fatal("entry should still exist after no-change EnsureServers")
	}
}

func TestSessionMCPRegistry_IncrementalDiff_RemoveOnly(t *testing.T) {
	reg := newSessionMCPRegistry(slog.Default())

	// Pre-populate entries.
	reg.entries["keep"] = &mcpEntry{
		server:   ClientMCPServer{Name: "keep", Type: "http", URL: "https://keep.com"},
		registry: tool.NewRegistry(),
	}
	reg.entries["remove"] = &mcpEntry{
		server:   ClientMCPServer{Name: "remove", Type: "http", URL: "https://remove.com"},
		registry: tool.NewRegistry(),
	}

	// EnsureServers with only "keep" — should remove "remove" without connecting anything.
	err := reg.EnsureServers(context.Background(), []ClientMCPServer{
		{Name: "keep", Type: "http", URL: "https://keep.com"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := reg.entries["keep"]; !ok {
		t.Fatal("'keep' entry should remain")
	}
	if _, ok := reg.entries["remove"]; ok {
		t.Fatal("'remove' entry should have been deleted")
	}
}

func TestSessionMCPRegistry_MCPInstructions_Empty(t *testing.T) {
	reg := newSessionMCPRegistry(slog.Default())
	reg.entries["srv"] = &mcpEntry{
		server:   ClientMCPServer{Name: "srv"},
		registry: tool.NewRegistry(), // no MCP sessions → no instructions
	}

	instrs := reg.MCPInstructions()
	if instrs != nil {
		t.Fatalf("expected nil instructions from empty registry, got %v", instrs)
	}

	reg.Close()
}

func TestSessionMCPRegistry_PingAll_Empty(t *testing.T) {
	reg := newSessionMCPRegistry(slog.Default())

	failed := reg.PingAll(context.Background())
	if len(failed) != 0 {
		t.Fatalf("expected no failures on empty registry, got %v", failed)
	}
}
