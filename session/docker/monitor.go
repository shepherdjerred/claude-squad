package docker

import (
	"bytes"
	"sync"
)

// statusMonitor tracks content changes for HasUpdated().
type statusMonitor struct {
	mu       sync.RWMutex
	lastHash []byte
	updated  bool
}

func newStatusMonitor() *statusMonitor {
	return &statusMonitor{}
}

func (m *statusMonitor) hasChanged(hash []byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if bytes.Equal(m.lastHash, hash) {
		return false
	}
	m.lastHash = hash
	return true
}

func (m *statusMonitor) markUpdated() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updated = true
}
