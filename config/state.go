package config

import (
	"claude-squad/log"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	StateFileName     = "state.json"
	InstancesFileName = "instances.json"
)

// InstanceStorage handles instance-related operations
type InstanceStorage interface {
	// SaveInstances saves the raw instance data
	SaveInstances(instancesJSON json.RawMessage) error
	// GetInstances returns the raw instance data
	GetInstances() json.RawMessage
	// DeleteAllInstances removes all stored instances
	DeleteAllInstances() error
}

// AppState handles application-level state
type AppState interface {
	// GetHelpScreensSeen returns the bitmask of seen help screens
	GetHelpScreensSeen() uint32
	// SetHelpScreensSeen updates the bitmask of seen help screens
	SetHelpScreensSeen(seen uint32) error
}

// StateManager combines instance storage and app state management
type StateManager interface {
	InstanceStorage
	AppState
}

// State represents the application state that persists between sessions
type State struct {
	// HelpScreensSeen is a bitmask tracking which help screens have been shown
	HelpScreensSeen uint32 `json:"help_screens_seen"`
	// Instances stores the serialized instance data as raw JSON
	InstancesData json.RawMessage `json:"instances"`

	// lastModTime tracks when we last read the state file (not serialized)
	lastModTime time.Time `json:"-"`
}

// DefaultState returns the default state
func DefaultState() *State {
	return &State{
		HelpScreensSeen: 0,
		InstancesData:   json.RawMessage("[]"),
	}
}

// LoadState loads the state from disk. If it cannot be done, we return the default state.
// This function acquires a shared lock to allow concurrent reads.
func LoadState() *State {
	configDir, err := GetConfigDir()
	if err != nil {
		log.ErrorLog.Printf("failed to get config directory: %v", err)
		return DefaultState()
	}

	statePath := filepath.Join(configDir, StateFileName)

	// Acquire shared lock for reading
	lock := NewFileLock(statePath)
	if err := lock.RLock(); err != nil {
		log.WarningLog.Printf("failed to acquire read lock: %v", err)
		// Continue without lock - better to have stale data than fail
	} else {
		defer lock.Unlock()
	}

	// Get file mod time before reading
	var modTime time.Time
	if info, err := os.Stat(statePath); err == nil {
		modTime = info.ModTime()
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create and save default state if file doesn't exist
			defaultState := DefaultState()
			defaultState.lastModTime = time.Now()
			if saveErr := SaveState(defaultState); saveErr != nil {
				log.WarningLog.Printf("failed to save default state: %v", saveErr)
			}
			return defaultState
		}

		log.WarningLog.Printf("failed to get state file: %v", err)
		return DefaultState()
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		log.ErrorLog.Printf("failed to parse state file: %v", err)
		return DefaultState()
	}

	state.lastModTime = modTime
	return &state
}

// SaveState saves the state to disk.
// This function acquires an exclusive lock to prevent concurrent writes.
func SaveState(state *State) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	statePath := filepath.Join(configDir, StateFileName)

	// Acquire exclusive lock for writing
	lock := NewFileLock(statePath)
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("failed to acquire write lock: %w", err)
	}
	defer lock.Unlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(statePath, data, 0644); err != nil {
		return err
	}

	// Update lastModTime after successful write
	if info, err := os.Stat(statePath); err == nil {
		state.lastModTime = info.ModTime()
	}

	return nil
}

// InstanceStorage interface implementation

// SaveInstances saves the raw instance data
func (s *State) SaveInstances(instancesJSON json.RawMessage) error {
	s.InstancesData = instancesJSON
	return SaveState(s)
}

// GetInstances returns the raw instance data
func (s *State) GetInstances() json.RawMessage {
	return s.InstancesData
}

// DeleteAllInstances removes all stored instances
func (s *State) DeleteAllInstances() error {
	s.InstancesData = json.RawMessage("[]")
	return SaveState(s)
}

// AppState interface implementation

// GetHelpScreensSeen returns the bitmask of seen help screens
func (s *State) GetHelpScreensSeen() uint32 {
	return s.HelpScreensSeen
}

// SetHelpScreensSeen updates the bitmask of seen help screens
func (s *State) SetHelpScreensSeen(seen uint32) error {
	s.HelpScreensSeen = seen
	return SaveState(s)
}

// State sync methods

// GetLastModTime returns the modification time when this state was last read from disk.
func (s *State) GetLastModTime() time.Time {
	return s.lastModTime
}

// GetStateModTime returns the current modification time of the state file on disk.
func GetStateModTime() (time.Time, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return time.Time{}, err
	}

	statePath := filepath.Join(configDir, StateFileName)
	info, err := os.Stat(statePath)
	if err != nil {
		return time.Time{}, err
	}

	return info.ModTime(), nil
}

// NeedsRefresh checks if the state file has been modified since the given time.
// Returns true if the file has been modified and should be refreshed.
func NeedsRefresh(since time.Time) bool {
	modTime, err := GetStateModTime()
	if err != nil {
		return false
	}
	return modTime.After(since)
}

// RefreshFromDisk reloads the state from disk if it has been modified.
// Returns true if the state was refreshed, false if no refresh was needed.
func (s *State) RefreshFromDisk() (bool, error) {
	if !NeedsRefresh(s.lastModTime) {
		return false, nil
	}

	configDir, err := GetConfigDir()
	if err != nil {
		return false, fmt.Errorf("failed to get config directory: %w", err)
	}

	statePath := filepath.Join(configDir, StateFileName)

	// Acquire shared lock for reading
	lock := NewFileLock(statePath)
	if err := lock.RLock(); err != nil {
		return false, fmt.Errorf("failed to acquire read lock: %w", err)
	}
	defer lock.Unlock()

	// Get current mod time
	info, err := os.Stat(statePath)
	if err != nil {
		return false, fmt.Errorf("failed to stat state file: %w", err)
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		return false, fmt.Errorf("failed to read state file: %w", err)
	}

	var newState State
	if err := json.Unmarshal(data, &newState); err != nil {
		return false, fmt.Errorf("failed to parse state file: %w", err)
	}

	// Update this state with the new data
	s.HelpScreensSeen = newState.HelpScreensSeen
	s.InstancesData = newState.InstancesData
	s.lastModTime = info.ModTime()

	return true, nil
}
