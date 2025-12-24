package docker

import (
	"sync"
	"time"
)

// contentCache provides a TTL-based cache for pane content.
type contentCache struct {
	mu         sync.RWMutex
	content    string
	lastUpdate time.Time
	ttl        time.Duration
}

func newContentCache(ttl time.Duration) *contentCache {
	return &contentCache{ttl: ttl}
}

func (c *contentCache) Get() (string, []byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.lastUpdate.IsZero() || time.Since(c.lastUpdate) > c.ttl {
		return "", nil, false
	}
	return c.content, nil, true
}

func (c *contentCache) Set(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.content = content
	c.lastUpdate = time.Now()
}

func (c *contentCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastUpdate = time.Time{}
}
