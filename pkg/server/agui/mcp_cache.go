package agui

import (
	"log/slog"
	"sync"
	"time"
)

const (
	mcpCacheTTL        = 10 * time.Minute
	mcpCacheMaxThreads = 200
)

// mcpCacheEntry holds a cached MCP registry for a thread.
type mcpCacheEntry struct {
	registry   *sessionMCPRegistry
	lastAccess time.Time
}

// threadMCPCache provides thread-level caching of MCP registries so that
// consecutive turns on the same thread reuse existing connections rather than
// reconnecting each time.
type threadMCPCache struct {
	mu      sync.Mutex
	entries map[string]*mcpCacheEntry
	logger  *slog.Logger
}

func newThreadMCPCache(logger *slog.Logger) *threadMCPCache {
	return &threadMCPCache{
		entries: make(map[string]*mcpCacheEntry),
		logger:  logger,
	}
}

// get returns the cached registry for a thread, or nil if not cached / expired.
func (c *threadMCPCache) get(threadID string) *sessionMCPRegistry {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[threadID]
	if !ok {
		return nil
	}
	if time.Since(entry.lastAccess) > mcpCacheTTL {
		entry.registry.Close()
		delete(c.entries, threadID)
		return nil
	}
	entry.lastAccess = time.Now()
	return entry.registry
}

// put stores a registry in the cache, evicting stale entries if needed.
func (c *threadMCPCache) put(threadID string, reg *sessionMCPRegistry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If replacing an existing entry for this thread, close the old one.
	if old, ok := c.entries[threadID]; ok && old.registry != reg {
		old.registry.Close()
	}

	c.entries[threadID] = &mcpCacheEntry{
		registry:   reg,
		lastAccess: time.Now(),
	}

	if len(c.entries) > mcpCacheMaxThreads {
		c.evictLocked()
	}
}

// remove removes and closes the registry for a thread.
func (c *threadMCPCache) remove(threadID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[threadID]; ok {
		entry.registry.Close()
		delete(c.entries, threadID)
	}
}

// closeAll closes all cached registries. Called during Gateway shutdown.
func (c *threadMCPCache) closeAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for id, entry := range c.entries {
		entry.registry.Close()
		delete(c.entries, id)
	}
}

// size returns the number of cached entries.
func (c *threadMCPCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *threadMCPCache) evictLocked() {
	now := time.Now()
	// First pass: remove expired entries.
	for id, entry := range c.entries {
		if now.Sub(entry.lastAccess) > mcpCacheTTL {
			entry.registry.Close()
			delete(c.entries, id)
			c.logger.Debug("mcp cache: evicted expired entry", "thread_id", id)
		}
	}
	// Second pass: if still over limit, evict oldest.
	for len(c.entries) > mcpCacheMaxThreads {
		var oldestID string
		var oldestTime time.Time
		for id, entry := range c.entries {
			if oldestID == "" || entry.lastAccess.Before(oldestTime) {
				oldestID = id
				oldestTime = entry.lastAccess
			}
		}
		if oldestID == "" {
			break
		}
		if entry, ok := c.entries[oldestID]; ok {
			entry.registry.Close()
		}
		delete(c.entries, oldestID)
		c.logger.Debug("mcp cache: evicted LRU entry", "thread_id", oldestID)
	}
}
