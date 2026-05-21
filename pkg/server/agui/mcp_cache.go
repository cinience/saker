package agui

import (
	"log/slog"
	"sync"
	"time"
)

const (
	mcpCacheTTL            = 10 * time.Minute
	mcpCacheMaxThreads     = 200
	mcpCacheCleanupInterval = 2 * time.Minute
)

// mcpCacheEntry holds a cached MCP registry for a thread.
type mcpCacheEntry struct {
	registry   *SessionMCPRegistry
	lastAccess time.Time
}

// ThreadMCPCache provides thread-level caching of MCP registries so that
// consecutive turns on the same thread reuse existing connections rather than
// reconnecting each time.
type ThreadMCPCache struct {
	mu      sync.Mutex
	entries map[string]*mcpCacheEntry
	logger  *slog.Logger
	stopCh  chan struct{}
}

func NewThreadMCPCache(logger *slog.Logger) *ThreadMCPCache {
	c := &ThreadMCPCache{
		entries: make(map[string]*mcpCacheEntry),
		logger:  logger,
		stopCh:  make(chan struct{}),
	}
	go c.cleanupLoop()
	return c
}

func (c *ThreadMCPCache) cleanupLoop() {
	ticker := time.NewTicker(mcpCacheCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.evictExpired()
		}
	}
}

func (c *ThreadMCPCache) evictExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for id, entry := range c.entries {
		if now.Sub(entry.lastAccess) > mcpCacheTTL {
			entry.registry.Close()
			delete(c.entries, id)
			c.logger.Debug("mcp cache: evicted expired entry", "thread_id", id)
		}
	}
}

// Get returns the cached registry for a thread, or nil if not cached / expired.
func (c *ThreadMCPCache) Get(threadID string) *SessionMCPRegistry {
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

// Put stores a registry in the cache, evicting stale entries if needed.
func (c *ThreadMCPCache) Put(threadID string, reg *SessionMCPRegistry) {
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
		c.evictOverflowLocked()
	}
}

// Remove removes and closes the registry for a thread.
func (c *ThreadMCPCache) Remove(threadID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[threadID]; ok {
		entry.registry.Close()
		delete(c.entries, threadID)
	}
}

// GetServerNames returns the connected server names for a thread, or nil.
func (c *ThreadMCPCache) GetServerNames(threadID string) []string {
	c.mu.Lock()
	entry, ok := c.entries[threadID]
	if !ok {
		c.mu.Unlock()
		return nil
	}
	reg := entry.registry
	c.mu.Unlock()
	return reg.ServerNames()
}

// CloseAll stops the cleanup goroutine and closes all cached registries.
// Safe to call multiple times.
func (c *ThreadMCPCache) CloseAll() {
	c.mu.Lock()
	if c.entries == nil {
		c.mu.Unlock()
		return
	}
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
	for id, entry := range c.entries {
		entry.registry.Close()
		delete(c.entries, id)
	}
	c.entries = nil
	c.mu.Unlock()
}

// Size returns the number of cached entries.
func (c *ThreadMCPCache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *ThreadMCPCache) evictOverflowLocked() {
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
