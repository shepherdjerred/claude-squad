package session

import (
	"claude-squad/config"
	"claude-squad/log"
	"encoding/json"
	"fmt"
	"time"
)

// InstanceData represents the serializable data of an Instance
type InstanceData struct {
	Title        string     `json:"title"`
	Path         string     `json:"path"`
	Branch       string     `json:"branch"`
	Status       Status     `json:"status"`
	Height       int        `json:"height"`
	Width        int        `json:"width"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastOpenedAt *time.Time `json:"last_opened_at,omitempty"`
	AutoYes      bool       `json:"auto_yes"`
	Archived     bool       `json:"archived"`

	Program          string          `json:"program"`
	Multiplexer      string          `json:"multiplexer"`
	Worktree         GitWorktreeData `json:"worktree"`
	DiffStats        DiffStatsData   `json:"diff_stats"`
	Summary          string          `json:"summary,omitempty"`
	SummaryUpdatedAt time.Time       `json:"summary_updated_at,omitempty"`

	// ClaudeSessionID is the Claude CLI session ID for resuming conversations after restart
	ClaudeSessionID string `json:"claude_session_id,omitempty"`

	// SessionType indicates the session type: "zellij", "docker-bind", or "docker-clone"
	SessionType string `json:"session_type,omitempty"`

	// DockerContainerID is the Docker container ID for Docker sessions
	DockerContainerID string `json:"docker_container_id,omitempty"`

	// DockerRepoURL is the git repo URL for docker-clone mode
	DockerRepoURL string `json:"docker_repo_url,omitempty"`
}

// GitWorktreeData represents the serializable data of a GitWorktree
type GitWorktreeData struct {
	RepoPath      string `json:"repo_path"`
	WorktreePath  string `json:"worktree_path"`
	SessionName   string `json:"session_name"`
	BranchName    string `json:"branch_name"`
	BaseCommitSHA string `json:"base_commit_sha"`
}

// DiffStatsData represents the serializable data of a DiffStats
type DiffStatsData struct {
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Content string `json:"content"`
}

// Storage handles saving and loading instances using the state interface
type Storage struct {
	state config.InstanceStorage
}

// NewStorage creates a new storage instance
func NewStorage(state config.InstanceStorage) (*Storage, error) {
	return &Storage{
		state: state,
	}, nil
}

// SaveInstances saves the list of instances to disk
func (s *Storage) SaveInstances(instances []*Instance) error {
	// Convert instances to InstanceData, deduplicating by title
	data := make([]InstanceData, 0)
	seenTitles := make(map[string]bool)
	for _, instance := range instances {
		if instance.Started() {
			instanceData := instance.ToInstanceData()
			// Skip duplicates - keep only the first instance with each title
			if seenTitles[instanceData.Title] {
				log.WarningLog.Printf("Skipping duplicate instance when saving: %s", instanceData.Title)
				continue
			}
			seenTitles[instanceData.Title] = true
			data = append(data, instanceData)
		}
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	return s.state.SaveInstances(jsonData)
}

// LoadInstances loads the list of instances from disk.
// Invalid instances (e.g., those whose multiplexer sessions no longer exist)
// are automatically filtered out and the cleaned state is saved back to disk.
func (s *Storage) LoadInstances() ([]*Instance, error) {
	jsonData := s.state.GetInstances()

	var instancesData []InstanceData
	if err := json.Unmarshal(jsonData, &instancesData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instances: %w", err)
	}

	instances := make([]*Instance, 0, len(instancesData))
	skippedCount := 0
	for _, data := range instancesData {
		instance, err := FromInstanceData(data)
		if err != nil {
			// Log warning and skip this instance instead of failing
			log.WarningLog.Printf("Skipping invalid instance %q: %v", data.Title, err)
			skippedCount++
			continue
		}
		instances = append(instances, instance)
	}

	// If any instances were filtered out, save the cleaned state
	if skippedCount > 0 {
		log.InfoLog.Printf("Removed %d invalid instance(s) from state", skippedCount)
		if err := s.SaveInstances(instances); err != nil {
			log.WarningLog.Printf("Failed to save cleaned state: %v", err)
		}
	}

	return instances, nil
}

// DeleteInstance removes an instance from storage
func (s *Storage) DeleteInstance(title string) error {
	instances, err := s.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	found := false
	newInstances := make([]*Instance, 0)
	for _, instance := range instances {
		data := instance.ToInstanceData()
		if data.Title != title {
			newInstances = append(newInstances, instance)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", title)
	}

	return s.SaveInstances(newInstances)
}

// UpdateInstance updates an existing instance in storage
func (s *Storage) UpdateInstance(instance *Instance) error {
	instances, err := s.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	data := instance.ToInstanceData()
	found := false
	for i, existing := range instances {
		existingData := existing.ToInstanceData()
		if existingData.Title == data.Title {
			instances[i] = instance
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", data.Title)
	}

	return s.SaveInstances(instances)
}

// DeleteAllInstances removes all stored instances
func (s *Storage) DeleteAllInstances() error {
	return s.state.DeleteAllInstances()
}

// ArchiveInstance archives an instance by title
func (s *Storage) ArchiveInstance(title string) error {
	return s.setInstanceArchived(title, true)
}

// UnarchiveInstance unarchives an instance by title
func (s *Storage) UnarchiveInstance(title string) error {
	return s.setInstanceArchived(title, false)
}

// setInstanceArchived sets the archived state of an instance
func (s *Storage) setInstanceArchived(title string, archived bool) error {
	jsonData := s.state.GetInstances()

	var instancesData []InstanceData
	if err := json.Unmarshal(jsonData, &instancesData); err != nil {
		return fmt.Errorf("failed to unmarshal instances: %w", err)
	}

	found := false
	for i := range instancesData {
		if instancesData[i].Title == title {
			instancesData[i].Archived = archived
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", title)
	}

	// Marshal and save
	jsonData, err := json.Marshal(instancesData)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	return s.state.SaveInstances(jsonData)
}

// StateSyncer is an optional interface for states that support sync from disk
type StateSyncer interface {
	RefreshFromDisk() (bool, error)
}

// SyncFromDisk checks if the state file has been modified by another process
// and reloads instances if needed. Returns the new instances and whether a sync occurred.
// The caller is responsible for merging these with any in-memory instances.
func (s *Storage) SyncFromDisk() ([]*Instance, bool, error) {
	// Check if the underlying state supports sync
	syncer, ok := s.state.(StateSyncer)
	if !ok {
		return nil, false, nil
	}

	// Try to refresh from disk
	refreshed, err := syncer.RefreshFromDisk()
	if err != nil {
		return nil, false, fmt.Errorf("failed to refresh state from disk: %w", err)
	}

	if !refreshed {
		return nil, false, nil
	}

	// State was refreshed, reload instances
	log.InfoLog.Printf("State file changed, reloading instances from disk")
	instances, err := s.LoadInstances()
	if err != nil {
		return nil, true, fmt.Errorf("failed to load instances after refresh: %w", err)
	}

	return instances, true, nil
}
