package config

import (
	"os"
	"path/filepath"
)

const lockFileName = "state.lock"

// FileLock provides file-based locking for cross-process synchronization.
// It uses a separate lock file rather than locking the data file directly.
type FileLock struct {
	path string
	file *os.File
}

// NewFileLock creates a new FileLock for the given path.
// The lock file will be created in the same directory as the given path.
func NewFileLock(path string) *FileLock {
	lockPath := filepath.Join(filepath.Dir(path), lockFileName)
	return &FileLock{
		path: lockPath,
	}
}

// GetStateLock returns a FileLock for the state file.
// This is a convenience function that creates a lock for the default state file location.
func GetStateLock() (*FileLock, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}
	statePath := filepath.Join(configDir, StateFileName)
	return NewFileLock(statePath), nil
}
