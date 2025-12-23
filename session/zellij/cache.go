package zellij

import (
	"sync"
	"time"
)

// contentCache provides a TTL-based cache for pane content to reduce
// redundant captures when content hasn't changed.
type contentCache struct {
	mu         sync.RWMutex
	content    string
	lastUpdate time.Time
	ttl        time.Duration
}

// newContentCache creates a new content cache with the specified TTL.
func newContentCache(ttl time.Duration) *contentCache {
	return &contentCache{
		ttl: ttl,
	}
}

// Get returns the cached content and whether it's still valid.
// Returns (content, hash, valid).
func (c *contentCache) Get() (string, []byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.lastUpdate.IsZero() {
		return "", nil, false
	}

	if time.Since(c.lastUpdate) > c.ttl {
		return "", nil, false
	}

	return c.content, nil, true
}

// Set updates the cached content with a new value.
func (c *contentCache) Set(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.content = content
	c.lastUpdate = time.Now()
}

// Invalidate clears the cache, forcing the next Get to return invalid.
func (c *contentCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastUpdate = time.Time{}
}

// IsStale returns true if the cache has expired.
func (c *contentCache) IsStale() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.lastUpdate.IsZero() {
		return true
	}
	return time.Since(c.lastUpdate) > c.ttl
}
