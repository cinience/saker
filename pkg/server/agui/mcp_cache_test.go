package agui

import (
	"log/slog"
	"testing"

	"github.com/saker-ai/saker/pkg/tool"
)

func TestThreadMCPCache_GetPut(t *testing.T) {
	cache := newThreadMCPCache(slog.Default())

	reg := newSessionMCPRegistry(slog.Default())
	reg.entries["srv1"] = &mcpEntry{
		server:   ClientMCPServer{Name: "srv1", Type: "http", URL: "https://example.com"},
		registry: tool.NewRegistry(),
	}

	cache.put("thread-1", reg)

	got := cache.get("thread-1")
	if got != reg {
		t.Fatal("expected to get back the same registry")
	}

	if cache.size() != 1 {
		t.Fatalf("expected size 1, got %d", cache.size())
	}
}

func TestThreadMCPCache_GetMiss(t *testing.T) {
	cache := newThreadMCPCache(slog.Default())

	got := cache.get("nonexistent")
	if got != nil {
		t.Fatal("expected nil for nonexistent thread")
	}
}

func TestThreadMCPCache_Remove(t *testing.T) {
	cache := newThreadMCPCache(slog.Default())

	reg := newSessionMCPRegistry(slog.Default())
	reg.entries["srv1"] = &mcpEntry{
		server:   ClientMCPServer{Name: "srv1"},
		registry: tool.NewRegistry(),
	}

	cache.put("thread-1", reg)
	cache.remove("thread-1")

	if cache.size() != 0 {
		t.Fatalf("expected size 0 after remove, got %d", cache.size())
	}
	if got := cache.get("thread-1"); got != nil {
		t.Fatal("expected nil after remove")
	}
}

func TestThreadMCPCache_CloseAll(t *testing.T) {
	cache := newThreadMCPCache(slog.Default())

	for i := 0; i < 3; i++ {
		reg := newSessionMCPRegistry(slog.Default())
		reg.entries["srv"] = &mcpEntry{
			server:   ClientMCPServer{Name: "srv"},
			registry: tool.NewRegistry(),
		}
		cache.put("thread-"+string(rune('a'+i)), reg)
	}

	if cache.size() != 3 {
		t.Fatalf("expected 3 entries, got %d", cache.size())
	}

	cache.closeAll()

	if cache.size() != 0 {
		t.Fatalf("expected 0 entries after closeAll, got %d", cache.size())
	}
}

func TestThreadMCPCache_PutReplacesOld(t *testing.T) {
	cache := newThreadMCPCache(slog.Default())

	reg1 := newSessionMCPRegistry(slog.Default())
	reg1.entries["srv1"] = &mcpEntry{
		server:   ClientMCPServer{Name: "srv1"},
		registry: tool.NewRegistry(),
	}
	cache.put("thread-1", reg1)

	reg2 := newSessionMCPRegistry(slog.Default())
	reg2.entries["srv2"] = &mcpEntry{
		server:   ClientMCPServer{Name: "srv2"},
		registry: tool.NewRegistry(),
	}
	cache.put("thread-1", reg2)

	got := cache.get("thread-1")
	if got != reg2 {
		t.Fatal("expected to get the new registry after replacement")
	}
	if cache.size() != 1 {
		t.Fatalf("expected size 1, got %d", cache.size())
	}
}
