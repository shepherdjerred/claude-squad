//go:build windows

package config

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// Lock acquires an exclusive lock on the file.
// This blocks until the lock is available.
func (l *FileLock) Lock() error {
	if l.file != nil {
		return fmt.Errorf("lock already held")
	}

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}

	// LockFileEx with LOCKFILE_EXCLUSIVE_LOCK for exclusive lock
	// We lock the entire file by using max values for length
	ol := new(windows.Overlapped)
	err = windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		1,
		0,
		ol,
	)
	if err != nil {
		f.Close()
		return fmt.Errorf("failed to acquire exclusive lock: %w", err)
	}

	l.file = f
	return nil
}

// RLock acquires a shared (read) lock on the file.
// Multiple processes can hold a shared lock simultaneously.
// This blocks until the lock is available.
func (l *FileLock) RLock() error {
	if l.file != nil {
		return fmt.Errorf("lock already held")
	}

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}

	// LockFileEx without LOCKFILE_EXCLUSIVE_LOCK for shared lock
	ol := new(windows.Overlapped)
	err = windows.LockFileEx(
		windows.Handle(f.Fd()),
		0, // No flags = shared lock
		0,
		1,
		0,
		ol,
	)
	if err != nil {
		f.Close()
		return fmt.Errorf("failed to acquire shared lock: %w", err)
	}

	l.file = f
	return nil
}

// Unlock releases the lock on the file.
func (l *FileLock) Unlock() error {
	if l.file == nil {
		return nil
	}

	ol := new(windows.Overlapped)
	err := windows.UnlockFileEx(
		windows.Handle(l.file.Fd()),
		0,
		1,
		0,
		ol,
	)
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	if err := l.file.Close(); err != nil {
		return fmt.Errorf("failed to close lock file: %w", err)
	}

	l.file = nil
	return nil
}
