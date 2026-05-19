package agui

import (
	"fmt"
	"testing"
	"time"

	"github.com/saker-ai/saker/pkg/server"
)

func TestArtifactCache_StoreAndLoad(t *testing.T) {
	t.Parallel()
	c := newArtifactCache()
	arts := []server.Artifact{
		{Type: "image", URL: "https://example.com/img.png", Name: "test"},
	}
	c.store("thread_1", arts)

	loaded := c.load("thread_1")
	if len(loaded) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(loaded))
	}
	if loaded[0].URL != "https://example.com/img.png" {
		t.Errorf("URL = %q", loaded[0].URL)
	}
}

func TestArtifactCache_LoadNonexistent(t *testing.T) {
	t.Parallel()
	c := newArtifactCache()
	loaded := c.load("nonexistent")
	if loaded != nil {
		t.Fatalf("expected nil for nonexistent thread, got %v", loaded)
	}
}

func TestArtifactCache_AppendOnStore(t *testing.T) {
	t.Parallel()
	c := newArtifactCache()
	c.store("thread_1", []server.Artifact{{Type: "image", URL: "a.png"}})
	c.store("thread_1", []server.Artifact{{Type: "video", URL: "b.mp4"}})

	loaded := c.load("thread_1")
	if len(loaded) != 2 {
		t.Fatalf("expected 2 artifacts after append, got %d", len(loaded))
	}
}

func TestArtifactCache_TTLExpiry(t *testing.T) {
	t.Parallel()
	c := newArtifactCache()
	c.store("thread_old", []server.Artifact{{Type: "image", URL: "old.png"}})

	// Manually backdate the entry.
	c.mu.Lock()
	c.entries["thread_old"].lastWrite = time.Now().Add(-artifactCacheTTL - time.Minute)
	c.mu.Unlock()

	loaded := c.load("thread_old")
	if loaded != nil {
		t.Fatalf("expired entry should return nil, got %v", loaded)
	}
}

func TestArtifactCache_EvictionOnCapacity(t *testing.T) {
	t.Parallel()
	c := newArtifactCache()

	// Fill cache beyond max.
	for i := 0; i < artifactCacheMaxThreads+10; i++ {
		c.store(fmt.Sprintf("thread_%d", i), []server.Artifact{{URL: "x.png"}})
	}

	c.mu.RLock()
	count := len(c.entries)
	c.mu.RUnlock()

	if count > artifactCacheMaxThreads {
		t.Errorf("cache should evict to capacity, got %d entries (max %d)", count, artifactCacheMaxThreads)
	}
}

func TestArtifactCache_EvictsStaleFirst(t *testing.T) {
	t.Parallel()
	c := newArtifactCache()

	// Add a stale entry and fill to capacity.
	c.store("stale_thread", []server.Artifact{{URL: "stale.png"}})
	c.mu.Lock()
	c.entries["stale_thread"].lastWrite = time.Now().Add(-artifactCacheTTL - time.Minute)
	c.mu.Unlock()

	for i := 0; i < artifactCacheMaxThreads; i++ {
		c.store(fmt.Sprintf("thread_%d", i), []server.Artifact{{URL: "x.png"}})
	}

	// Stale entry should have been evicted.
	loaded := c.load("stale_thread")
	if loaded != nil {
		t.Error("stale entry should be evicted first")
	}
}
