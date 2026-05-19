package agui

import (
	"sync"
	"time"

	"github.com/saker-ai/saker/pkg/server"
)

const artifactCacheTTL = 30 * time.Minute
const artifactCacheMaxThreads = 1000

type artifactEntry struct {
	artifacts []server.Artifact
	lastWrite time.Time
}

type artifactCache struct {
	mu      sync.RWMutex
	entries map[string]*artifactEntry
}

func newArtifactCache() artifactCache {
	return artifactCache{entries: make(map[string]*artifactEntry)}
}

func (c *artifactCache) store(threadID string, arts []server.Artifact) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[threadID]
	if !ok {
		e = &artifactEntry{}
		c.entries[threadID] = e
	}
	e.artifacts = append(e.artifacts, arts...)
	e.lastWrite = time.Now()

	// Evict stale entries if cache grows too large.
	if len(c.entries) > artifactCacheMaxThreads {
		c.evictLocked()
	}
}

func (c *artifactCache) load(threadID string) []server.Artifact {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[threadID]
	if !ok {
		return nil
	}
	if time.Since(e.lastWrite) > artifactCacheTTL {
		return nil
	}
	return e.artifacts
}

func (c *artifactCache) evictLocked() {
	now := time.Now()
	for id, e := range c.entries {
		if now.Sub(e.lastWrite) > artifactCacheTTL {
			delete(c.entries, id)
		}
	}
	// If still over limit, evict oldest entries.
	for len(c.entries) > artifactCacheMaxThreads {
		var oldestID string
		var oldestTime time.Time
		for id, e := range c.entries {
			if oldestID == "" || e.lastWrite.Before(oldestTime) {
				oldestID = id
				oldestTime = e.lastWrite
			}
		}
		if oldestID != "" {
			delete(c.entries, oldestID)
		}
	}
}
