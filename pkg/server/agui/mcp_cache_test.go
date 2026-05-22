package agui

import (
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/saker-ai/saker/pkg/tool"
)

func TestThreadMCPCache_GetPut(t *testing.T) {
	cache := NewThreadMCPCache(slog.Default())

	reg := NewSessionMCPRegistry(slog.Default())
	reg.entries["srv1"] = &mcpEntry{
		server:   ClientMCPServer{Name: "srv1", Type: "http", URL: "https://example.com"},
		registry: tool.NewRegistry(),
	}

	cache.Put("thread-1", reg)

	got := cache.Get("thread-1")
	if got != reg {
		t.Fatal("expected to get back the same registry")
	}

	if cache.Size() != 1 {
		t.Fatalf("expected size 1, got %d", cache.Size())
	}
}

func TestThreadMCPCache_GetMiss(t *testing.T) {
	cache := NewThreadMCPCache(slog.Default())

	got := cache.Get("nonexistent")
	if got != nil {
		t.Fatal("expected nil for nonexistent thread")
	}
}

func TestThreadMCPCache_Remove(t *testing.T) {
	cache := NewThreadMCPCache(slog.Default())

	reg := NewSessionMCPRegistry(slog.Default())
	reg.entries["srv1"] = &mcpEntry{
		server:   ClientMCPServer{Name: "srv1"},
		registry: tool.NewRegistry(),
	}

	cache.Put("thread-1", reg)
	cache.Remove("thread-1")

	if cache.Size() != 0 {
		t.Fatalf("expected size 0 after remove, got %d", cache.Size())
	}
	if got := cache.Get("thread-1"); got != nil {
		t.Fatal("expected nil after remove")
	}
}

func TestThreadMCPCache_CloseAll(t *testing.T) {
	cache := NewThreadMCPCache(slog.Default())

	for i := 0; i < 3; i++ {
		reg := NewSessionMCPRegistry(slog.Default())
		reg.entries["srv"] = &mcpEntry{
			server:   ClientMCPServer{Name: "srv"},
			registry: tool.NewRegistry(),
		}
		cache.Put("thread-"+string(rune('a'+i)), reg)
	}

	if cache.Size() != 3 {
		t.Fatalf("expected 3 entries, got %d", cache.Size())
	}

	cache.CloseAll()

	if cache.Size() != 0 {
		t.Fatalf("expected 0 entries after closeAll, got %d", cache.Size())
	}
}

func TestThreadMCPCache_PutReplacesOld(t *testing.T) {
	cache := NewThreadMCPCache(slog.Default())

	reg1 := NewSessionMCPRegistry(slog.Default())
	reg1.entries["srv1"] = &mcpEntry{
		server:   ClientMCPServer{Name: "srv1"},
		registry: tool.NewRegistry(),
	}
	cache.Put("thread-1", reg1)

	reg2 := NewSessionMCPRegistry(slog.Default())
	reg2.entries["srv2"] = &mcpEntry{
		server:   ClientMCPServer{Name: "srv2"},
		registry: tool.NewRegistry(),
	}
	cache.Put("thread-1", reg2)

	got := cache.Get("thread-1")
	if got != reg2 {
		t.Fatal("expected to get the new registry after replacement")
	}
	if cache.Size() != 1 {
		t.Fatalf("expected size 1, got %d", cache.Size())
	}
}

func TestThreadMCPCache_ConcurrentReadWrite(t *testing.T) {
	cache := NewThreadMCPCache(slog.Default())
	defer cache.CloseAll()

	// Pre-populate a few entries.
	for i := 0; i < 5; i++ {
		reg := NewSessionMCPRegistry(slog.Default())
		reg.entries["srv"] = &mcpEntry{
			server:   ClientMCPServer{Name: "srv"},
			registry: tool.NewRegistry(),
		}
		cache.Put(fmt.Sprintf("thread-%d", i), reg)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	time.AfterFunc(200*time.Millisecond, func() { close(stop) })

	// 10 reader goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("thread-%d", id%5)
			for {
				select {
				case <-stop:
					return
				default:
				}
				cache.Get(key)
				cache.GetServerNames(key)
				cache.Size()
			}
		}(i)
	}

	// 2 writer goroutines
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for n := 0; ; n++ {
				select {
				case <-stop:
					return
				default:
				}
				key := fmt.Sprintf("thread-w%d-%d", id, n%3)
				reg := NewSessionMCPRegistry(slog.Default())
				reg.entries["srv"] = &mcpEntry{
					server:   ClientMCPServer{Name: "srv"},
					registry: tool.NewRegistry(),
				}
				cache.Put(key, reg)
				if n%2 == 0 {
					cache.Remove(key)
				}
			}
		}(i)
	}

	wg.Wait()
}
