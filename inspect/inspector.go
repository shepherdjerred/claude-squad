// Package inspect provides UI introspection for debugging and automated testing.
// It allows Claude Code (and other tools) to understand UI state without visual access.
package inspect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Introspectable is implemented by UI components that can report their state.
type Introspectable interface {
	// InspectNode returns a structured representation of this component.
	InspectNode() *Node
}

// Global state
var (
	enabled     bool
	enabledOnce sync.Once
	inspectFile string
)

// IsEnabled returns true if inspection mode is active.
func IsEnabled() bool {
	enabledOnce.Do(func() {
		enabled = os.Getenv("CLAUDE_SQUAD_INSPECT") == "1"
		if enabled {
			inspectFile = filepath.Join(os.TempDir(), "claudesquad-inspect.json")
		}
	})
	return enabled
}

// GetInspectFile returns the path to the inspection output file.
func GetInspectFile() string {
	if !IsEnabled() {
		return ""
	}
	return inspectFile
}

// WriteSnapshot writes a snapshot to the inspection file.
func WriteSnapshot(snapshot *Snapshot) error {
	if !IsEnabled() {
		return nil
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	if err := os.WriteFile(inspectFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
	}

	return nil
}

// WriteSnapshotToPath writes a snapshot to a specific path.
func WriteSnapshotToPath(snapshot *Snapshot, path string) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
	}

	return nil
}
