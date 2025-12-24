package session

import (
	"claude-squad/config"
	"claude-squad/log"
	"claude-squad/session/git"
	"claude-squad/session/zellij"
	"errors"
	"path/filepath"

	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
)

type Status int

const (
	// Running is the status when the instance is running and claude is working.
	Running Status = iota
	// Ready is if the claude instance is ready to be interacted with (waiting for user input).
	Ready
	// Loading is if the instance is loading (if we are starting it up or something).
	Loading
	// Paused is if the instance is paused (worktree removed but branch preserved).
	Paused
)

// Instance is a running instance of claude code.
type Instance struct {
	// Title is the title of the instance.
	Title string
	// Path is the path to the workspace.
	Path string
	// Branch is the branch of the instance.
	Branch string
	// Status is the status of the instance.
	Status Status
	// Program is the program to run in the instance.
	Program string
	// Height is the height of the instance.
	Height int
	// Width is the width of the instance.
	Width int
	// CreatedAt is the time the instance was created.
	CreatedAt time.Time
	// UpdatedAt is the time the instance was last updated.
	UpdatedAt time.Time
	// LastOpenedAt is the time the user last attached to this instance.
	LastOpenedAt *time.Time
	// AutoYes is true if the instance should automatically press enter when prompted.
	AutoYes bool
	// Prompt is the initial prompt to pass to the instance on startup
	Prompt string
	// Archived is true if the instance has been archived (hidden but not deleted).
	Archived bool

	// DiffStats stores the current git diff statistics
	diffStats *git.DiffStats

	// Summary is a short AI-generated description of the current session state
	Summary string
	// SummaryUpdatedAt is when the summary was last updated
	SummaryUpdatedAt time.Time

	// Background diff calculation timing
	lastDiffUpdate time.Time // When diff was last calculated
	lastActivity   time.Time // When instance status last changed

	// ClaudeSessionID is the Claude CLI session ID for resuming conversations after restart.
	// This is captured from Claude's project files after Claude starts.
	ClaudeSessionID string

	// SessionType indicates the type of session: "zellij", "docker-bind", or "docker-clone"
	SessionType string
	// DockerContainerID is the Docker container ID for Docker sessions
	DockerContainerID string
	// DockerRepoURL is the git repo URL for docker-clone mode
	DockerRepoURL string
	// DockerBaseImage is the Docker base image used for this session
	DockerBaseImage string

	// The below fields are initialized upon calling Start().

	started bool
	// session is the multiplexer session for the instance.
	session Multiplexer
	// multiplexerType is the type of multiplexer used for this instance.
	// Deprecated: Use SessionType instead.
	multiplexerType MultiplexerType
	// gitWorktree is the git worktree for the instance.
	gitWorktree *git.GitWorktree
}

// ToInstanceData converts an Instance to its serializable form
func (i *Instance) ToInstanceData() InstanceData {
	data := InstanceData{
		Title:             i.Title,
		Path:              i.Path,
		Branch:            i.Branch,
		Status:            i.Status,
		Height:            i.Height,
		Width:             i.Width,
		CreatedAt:         i.CreatedAt,
		UpdatedAt:         time.Now(),
		LastOpenedAt:      i.LastOpenedAt,
		Program:           i.Program,
		AutoYes:           i.AutoYes,
		Archived:          i.Archived,
		Multiplexer:       string(i.multiplexerType),
		Summary:           i.Summary,
		SummaryUpdatedAt:  i.SummaryUpdatedAt,
		ClaudeSessionID:   i.ClaudeSessionID,
		SessionType:       i.SessionType,
		DockerContainerID: i.DockerContainerID,
		DockerRepoURL:     i.DockerRepoURL,
	}

	// Only include worktree data if gitWorktree is initialized
	if i.gitWorktree != nil {
		data.Worktree = GitWorktreeData{
			RepoPath:      i.gitWorktree.GetRepoPath(),
			WorktreePath:  i.gitWorktree.GetWorktreePath(),
			SessionName:   i.gitWorktree.GetSessionName(),
			BranchName:    i.gitWorktree.GetBranchName(),
			BaseCommitSHA: i.gitWorktree.GetBaseCommitSHA(),
		}
	}

	// Only include diff stats if they exist
	if i.diffStats != nil {
		data.DiffStats = DiffStatsData{
			Added:   i.diffStats.Added,
			Removed: i.diffStats.Removed,
			Content: i.diffStats.Content,
		}
	}

	return data
}

// NewInstanceFromOrphan creates an Instance from recovered orphaned session data
func NewInstanceFromOrphan(orphan *zellij.OrphanedSession) (*Instance, error) {
	if orphan == nil {
		return nil, fmt.Errorf("orphan session data is nil")
	}

	// Validate required fields
	if orphan.SessionName == "" {
		return nil, fmt.Errorf("orphan session name is empty")
	}
	if orphan.WorktreePath == "" {
		return nil, fmt.Errorf("orphan worktree path is empty")
	}

	// Determine repo path - use recovered path or fall back to worktree path
	repoPath := orphan.RepoPath
	if repoPath == "" {
		repoPath = orphan.WorktreePath
	}

	// Create git worktree from recovered data
	gitWorktree := git.NewGitWorktreeFromStorage(
		repoPath,
		orphan.WorktreePath,
		orphan.Title,
		orphan.BranchName,
		"", // Base commit SHA is unknown for orphaned sessions
	)

	// Create the instance
	now := time.Now()
	instance := &Instance{
		Title:           orphan.Title,
		Path:            orphan.WorktreePath,
		Branch:          orphan.BranchName,
		Status:          Running,
		Program:         orphan.Program,
		Height:          0,
		Width:           0,
		CreatedAt:       now,
		UpdatedAt:       now,
		AutoYes:         false,
		SessionType:     config.SessionTypeZellij, // Orphaned sessions are always Zellij
		multiplexerType: MultiplexerZellij,
		gitWorktree:     gitWorktree,
	}

	// Create Zellij session and restore connection to existing session
	// Use the session name from gitWorktree for consistency
	session := NewMultiplexer(config.SessionTypeZellij, instance.gitWorktree.GetSessionName(), instance.Program, MultiplexerOptions{})
	instance.session = session

	// Restore connection to existing session
	if err := session.Restore(); err != nil {
		return nil, fmt.Errorf("failed to restore orphan session: %w", err)
	}

	instance.started = true

	return instance, nil
}

// FromInstanceData creates a new Instance from serialized data
func FromInstanceData(data InstanceData) (*Instance, error) {
	// For backwards compatibility, default to zellij if no session type
	sessionType := data.SessionType
	if sessionType == "" {
		sessionType = config.SessionTypeZellij
	}
	mtype := MultiplexerZellij

	instance := &Instance{
		Title:             data.Title,
		Path:              data.Path,
		Branch:            data.Branch,
		Status:            data.Status,
		Height:            data.Height,
		Width:             data.Width,
		CreatedAt:         data.CreatedAt,
		UpdatedAt:         data.UpdatedAt,
		LastOpenedAt:      data.LastOpenedAt,
		Program:           data.Program,
		Archived:          data.Archived,
		Summary:           data.Summary,
		SummaryUpdatedAt:  data.SummaryUpdatedAt,
		ClaudeSessionID:   data.ClaudeSessionID,
		SessionType:       sessionType,
		DockerContainerID: data.DockerContainerID,
		DockerRepoURL:     data.DockerRepoURL,
		multiplexerType:   mtype,
		diffStats: &git.DiffStats{
			Added:   data.DiffStats.Added,
			Removed: data.DiffStats.Removed,
			Content: data.DiffStats.Content,
		},
	}

	// For Docker clone mode, we may not have a worktree
	if data.Worktree.WorktreePath != "" || sessionType != config.SessionTypeDockerClone {
		instance.gitWorktree = git.NewGitWorktreeFromStorage(
			data.Worktree.RepoPath,
			data.Worktree.WorktreePath,
			data.Worktree.SessionName,
			data.Worktree.BranchName,
			data.Worktree.BaseCommitSHA,
		)
	}

	if instance.Paused() || instance.Archived {
		instance.started = true
		// Create session based on session type
		sessionName := data.Title
		if instance.gitWorktree != nil {
			sessionName = instance.gitWorktree.GetSessionName()
		}
		instance.session = NewMultiplexer(sessionType, sessionName, instance.Program, MultiplexerOptions{
			BaseImage:  instance.DockerBaseImage,
			RepoURL:    instance.DockerRepoURL,
			BranchName: instance.Branch,
		})
	} else {
		if err := instance.Start(false); err != nil {
			return nil, err
		}
	}

	return instance, nil
}

// Options for creating a new instance
type InstanceOptions struct {
	// Title is the title of the instance.
	Title string
	// Path is the path to the workspace.
	Path string
	// Program is the program to run in the instance (e.g. "claude", "aider --model ollama_chat/gemma3:1b")
	Program string
	// If AutoYes is true, then
	AutoYes bool
	// Multiplexer is deprecated and ignored. Use SessionType instead.
	// This field is kept for backwards compatibility.
	Multiplexer string
	// SessionType indicates the type of session: "zellij", "docker-bind", or "docker-clone"
	SessionType string
	// DockerBaseImage is the Docker base image for Docker sessions (e.g., "ubuntu:24.04")
	DockerBaseImage string
	// DockerRepoURL is the git repo URL for docker-clone mode
	DockerRepoURL string
}

func NewInstance(opts InstanceOptions) (*Instance, error) {
	t := time.Now()

	// Convert path to absolute
	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Default to zellij if no session type specified
	sessionType := opts.SessionType
	if sessionType == "" {
		sessionType = config.SessionTypeZellij
	}

	// For backwards compatibility, set multiplexerType based on session type
	muxType := MultiplexerZellij

	return &Instance{
		Title:           opts.Title,
		Status:          Ready,
		Path:            absPath,
		Program:         opts.Program,
		Height:          0,
		Width:           0,
		CreatedAt:       t,
		UpdatedAt:       t,
		AutoYes:         opts.AutoYes,
		multiplexerType: muxType,
		SessionType:     sessionType,
		DockerBaseImage: opts.DockerBaseImage,
		DockerRepoURL:   opts.DockerRepoURL,
	}, nil
}

func (i *Instance) RepoName() (string, error) {
	if !i.started {
		return "", fmt.Errorf("cannot get repo name for instance that has not been started")
	}
	return i.gitWorktree.GetRepoName(), nil
}

func (i *Instance) SetStatus(status Status) {
	i.Status = status
	i.lastActivity = time.Now()
}

// StartWithProgress starts the instance with an optional progress callback.
// The callback receives status messages during setup.
func (i *Instance) StartWithProgress(firstTimeSetup bool, progressCallback git.ProgressCallback) error {
	return i.startInternal(firstTimeSetup, progressCallback)
}

// firstTimeSetup is true if this is a new instance. Otherwise, it's one loaded from storage.
func (i *Instance) Start(firstTimeSetup bool) error {
	return i.startInternal(firstTimeSetup, nil)
}

func (i *Instance) startInternal(firstTimeSetup bool, progressCallback git.ProgressCallback) error {
	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	// Default session type to zellij for backwards compatibility
	if i.SessionType == "" {
		i.SessionType = config.SessionTypeZellij
	}
	if i.multiplexerType == "" {
		i.multiplexerType = MultiplexerZellij
	}

	// For Docker clone mode, we skip worktree setup (repo is cloned inside container)
	isDockerClone := i.SessionType == config.SessionTypeDockerClone

	if firstTimeSetup && !isDockerClone {
		// Create git worktree for Zellij and Docker bind-mount modes
		gitWorktree, branchName, err := git.NewGitWorktree(i.Path, i.Title)
		if err != nil {
			return fmt.Errorf("failed to create git worktree: %w", err)
		}
		i.gitWorktree = gitWorktree
		i.Branch = branchName
		// Set progress callback if provided
		if progressCallback != nil {
			i.gitWorktree.SetProgressCallback(progressCallback)
		}
	} else if firstTimeSetup && isDockerClone {
		// For Docker clone mode, just set up the branch name
		// The repo will be cloned inside the container
		i.Branch = i.Title // Branch name will be the instance title
	}

	// Create the multiplexer session
	var session Multiplexer
	if i.session != nil {
		// Use existing session (useful for testing)
		session = i.session
	} else {
		// Determine session name
		sessionName := i.Title
		if i.gitWorktree != nil {
			sessionName = i.gitWorktree.GetSessionName()
		}

		// Create new session using factory
		session = NewMultiplexer(i.SessionType, sessionName, i.Program, MultiplexerOptions{
			BaseImage:  i.DockerBaseImage,
			RepoURL:    i.DockerRepoURL,
			BranchName: i.Branch,
			WorkDir:    i.Path,
		})
	}
	i.session = session

	// Setup error handler to cleanup resources on any error
	var setupErr error
	defer func() {
		if setupErr != nil {
			if cleanupErr := i.Kill(); cleanupErr != nil {
				setupErr = fmt.Errorf("%v (cleanup error: %v)", setupErr, cleanupErr)
			}
		} else {
			i.started = true
		}
	}()

	if !firstTimeSetup {
		// Reuse existing session
		if err := session.Restore(); err != nil {
			setupErr = fmt.Errorf("failed to restore existing session: %w", err)
			return setupErr
		}
	} else {
		// For Docker clone mode, we don't have a git worktree - repo is cloned inside container
		if i.gitWorktree != nil {
			// Setup git worktree for Zellij and Docker bind-mount modes
			if err := i.gitWorktree.Setup(); err != nil {
				setupErr = fmt.Errorf("failed to setup git worktree: %w", err)
				return setupErr
			}
		}

		// Report progress for session start
		if progressCallback != nil {
			progressCallback("Starting terminal session...")
		}

		// Determine work directory for the session
		workDir := i.Path
		if i.gitWorktree != nil {
			workDir = i.gitWorktree.GetWorktreePath()
		}

		// Create new session
		if err := i.session.Start(workDir); err != nil {
			// Cleanup git worktree if session creation fails
			if i.gitWorktree != nil {
				if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
					err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
				}
			}
			setupErr = fmt.Errorf("failed to start new session: %w", err)
			return setupErr
		}
	}

	i.SetStatus(Running)

	return nil
}

// Kill terminates the instance and cleans up all resources
func (i *Instance) Kill() error {
	if !i.started {
		// If instance was never started, just return success
		return nil
	}

	var errs []error

	// Always try to cleanup both resources, even if one fails
	// Clean up session first since it's using the git worktree
	if i.session != nil {
		if err := i.session.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close session: %w", err))
		}
	}

	// Then clean up git worktree
	if i.gitWorktree != nil {
		if err := i.gitWorktree.Cleanup(); err != nil {
			errs = append(errs, fmt.Errorf("failed to cleanup git worktree: %w", err))
		}
	}

	return i.combineErrors(errs)
}

// combineErrors combines multiple errors into a single error
func (i *Instance) combineErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}

	errMsg := "multiple cleanup errors occurred:"
	for _, err := range errs {
		errMsg += "\n  - " + err.Error()
	}
	return fmt.Errorf("%s", errMsg)
}

func (i *Instance) Preview() (string, error) {
	if !i.started || i.Status == Paused {
		return "", nil
	}
	return i.session.CapturePaneContent()
}

func (i *Instance) HasUpdated() (updated bool, hasPrompt bool) {
	if !i.started {
		return false, false
	}
	return i.session.HasUpdated()
}

// TapEnter sends an enter key press to the session if AutoYes is enabled.
func (i *Instance) TapEnter() {
	if !i.started || !i.AutoYes {
		return
	}
	if err := i.session.TapEnter(); err != nil {
		log.ErrorLog.Printf("error tapping enter: %v", err)
	}
}

func (i *Instance) Attach() (chan struct{}, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot attach instance that has not been started")
	}
	// Track when the user last opened this instance
	now := time.Now()
	i.LastOpenedAt = &now
	return i.session.Attach()
}

func (i *Instance) SetPreviewSize(width, height int) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot set preview size for instance that has not been started or " +
			"is paused")
	}
	return i.session.SetDetachedSize(width, height)
}

// GetGitWorktree returns the git worktree for the instance
func (i *Instance) GetGitWorktree() (*git.GitWorktree, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot get git worktree for instance that has not been started")
	}
	return i.gitWorktree, nil
}

func (i *Instance) Started() bool {
	return i.started
}

// SetTitle sets the title of the instance. Returns an error if the instance has started.
// We cant change the title once it's been used for a session etc.
func (i *Instance) SetTitle(title string) error {
	if i.started {
		return fmt.Errorf("cannot change title of a started instance")
	}
	i.Title = title
	return nil
}

// Rename changes the display title of the instance. Unlike SetTitle, this can be called
// after the instance has started. Note that this only changes the display name - the
// underlying session name and git worktree path remain unchanged.
func (i *Instance) Rename(newTitle string) error {
	if newTitle == "" {
		return fmt.Errorf("title cannot be empty")
	}
	if len(newTitle) > 32 {
		return fmt.Errorf("title cannot be longer than 32 characters")
	}
	i.Title = newTitle
	i.UpdatedAt = time.Now()
	return nil
}

func (i *Instance) Paused() bool {
	return i.Status == Paused
}

// SessionAlive returns true if the multiplexer session is alive. This is a sanity check before attaching.
func (i *Instance) SessionAlive() bool {
	if i.session == nil {
		return false
	}
	return i.session.DoesSessionExist()
}

// Pause stops the session and removes the worktree, preserving the branch
func (i *Instance) Pause() error {
	if !i.started {
		return fmt.Errorf("cannot pause instance that has not been started")
	}
	if i.Status == Paused {
		return fmt.Errorf("instance is already paused")
	}

	var errs []error

	// Check if there are any changes to commit
	if dirty, err := i.gitWorktree.IsDirty(); err != nil {
		errs = append(errs, fmt.Errorf("failed to check if worktree is dirty: %w", err))
		log.ErrorLog.Print(err)
	} else if dirty {
		// Commit changes locally (without pushing to GitHub)
		commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s (paused)", i.Title, time.Now().Format(time.RFC822))
		if err := i.gitWorktree.CommitChanges(commitMsg); err != nil {
			errs = append(errs, fmt.Errorf("failed to commit changes: %w", err))
			log.ErrorLog.Print(err)
			// Return early if we can't commit changes to avoid corrupted state
			return i.combineErrors(errs)
		}
	}

	// Detach from session instead of closing to preserve session output
	if err := i.session.DetachSafely(); err != nil {
		errs = append(errs, fmt.Errorf("failed to detach session: %w", err))
		log.ErrorLog.Print(err)
		// Continue with pause process even if detach fails
	}

	// Check if worktree exists before trying to remove it
	if _, err := os.Stat(i.gitWorktree.GetWorktreePath()); err == nil {
		// Remove worktree but keep branch
		if err := i.gitWorktree.Remove(); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove git worktree: %w", err))
			log.ErrorLog.Print(err)
			return i.combineErrors(errs)
		}

		// Only prune if remove was successful
		if err := i.gitWorktree.Prune(); err != nil {
			errs = append(errs, fmt.Errorf("failed to prune git worktrees: %w", err))
			log.ErrorLog.Print(err)
			return i.combineErrors(errs)
		}
	}

	if err := i.combineErrors(errs); err != nil {
		log.ErrorLog.Print(err)
		return err
	}

	i.SetStatus(Paused)
	_ = clipboard.WriteAll(i.gitWorktree.GetBranchName())
	return nil
}

// Resume recreates the worktree and restarts the session
func (i *Instance) Resume() error {
	if !i.started {
		return fmt.Errorf("cannot resume instance that has not been started")
	}
	if i.Status != Paused {
		return fmt.Errorf("can only resume paused instances")
	}

	// Check if branch is checked out
	if checked, err := i.gitWorktree.IsBranchCheckedOut(); err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to check if branch is checked out: %w", err)
	} else if checked {
		return fmt.Errorf("cannot resume: branch is checked out, please switch to a different branch")
	}

	// Setup git worktree
	if err := i.gitWorktree.Setup(); err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to setup git worktree: %w", err)
	}

	// Check if session still exists from pause, otherwise create new one
	if i.session != nil && i.session.DoesSessionExist() {
		// Session exists, just restore PTY connection to it
		if err := i.session.Restore(); err != nil {
			log.ErrorLog.Print(err)
			// If restore fails, fall back to creating new session
			if err := i.session.Start(i.gitWorktree.GetWorktreePath()); err != nil {
				log.ErrorLog.Print(err)
				// Cleanup git worktree if session creation fails
				if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
					err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
					log.ErrorLog.Print(err)
				}
				return fmt.Errorf("failed to start new session: %w", err)
			}
		}
	} else {
		// Create new session
		if err := i.session.Start(i.gitWorktree.GetWorktreePath()); err != nil {
			log.ErrorLog.Print(err)
			// Cleanup git worktree if session creation fails
			if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
				log.ErrorLog.Print(err)
			}
			return fmt.Errorf("failed to start new session: %w", err)
		}
	}

	i.SetStatus(Running)
	return nil
}

// UpdateDiffStats updates the git diff statistics for this instance
func (i *Instance) UpdateDiffStats() error {
	if !i.started {
		i.diffStats = nil
		return nil
	}

	if i.Status == Paused {
		// Keep the previous diff stats if the instance is paused
		return nil
	}

	stats := i.gitWorktree.Diff()
	if stats.Error != nil {
		if strings.Contains(stats.Error.Error(), "base commit SHA not set") {
			// Worktree is not fully set up yet, not an error
			i.diffStats = nil
			return nil
		}
		return fmt.Errorf("failed to get diff stats: %w", stats.Error)
	}

	i.diffStats = stats
	i.lastDiffUpdate = time.Now()
	return nil
}

// GetDiffStats returns the current git diff statistics
func (i *Instance) GetDiffStats() *git.DiffStats {
	return i.diffStats
}

// ShouldUpdateDiff returns true if the instance is due for a diff stats update.
// Rate limiting: at least 10s since last activity, at most once per 30s.
func (i *Instance) ShouldUpdateDiff() bool {
	if !i.started || i.Status == Paused {
		return false
	}
	now := time.Now()
	// At least 30s since last diff calculation
	if !i.lastDiffUpdate.IsZero() && now.Sub(i.lastDiffUpdate) < 30*time.Second {
		return false
	}
	// At least 10s since last activity (status change)
	if !i.lastActivity.IsZero() && now.Sub(i.lastActivity) < 10*time.Second {
		return false
	}
	return true
}

// SendPrompt sends a prompt to the session
func (i *Instance) SendPrompt(prompt string) error {
	if !i.started {
		return fmt.Errorf("instance not started")
	}
	if i.session == nil {
		return fmt.Errorf("session not initialized")
	}
	if err := i.session.SendKeys(prompt); err != nil {
		return fmt.Errorf("error sending keys to session: %w", err)
	}

	// Brief pause to prevent carriage return from being interpreted as newline
	time.Sleep(100 * time.Millisecond)
	if err := i.session.TapEnter(); err != nil {
		return fmt.Errorf("error tapping enter: %w", err)
	}

	return nil
}

// PreviewFullHistory captures the entire pane output including full scrollback history
func (i *Instance) PreviewFullHistory() (string, error) {
	if !i.started || i.Status == Paused {
		return "", nil
	}
	return i.session.CapturePaneContentWithOptions("-", "-")
}

// SetSession sets the multiplexer session for testing purposes
func (i *Instance) SetSession(session Multiplexer) {
	i.session = session
}

// MarkAsStartedForTesting marks the instance as started for testing purposes.
// This allows tests to bypass the real session startup while still testing
// preview functionality.
func (i *Instance) MarkAsStartedForTesting() {
	i.started = true
}

// SendKeys sends keys to the session
func (i *Instance) SendKeys(keys string) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot send keys to instance that has not been started or is paused")
	}
	return i.session.SendKeys(keys)
}

// GetMultiplexerType returns the type of multiplexer used for this instance
// Deprecated: Use GetSessionType instead.
func (i *Instance) GetMultiplexerType() MultiplexerType {
	return i.multiplexerType
}

// GetSessionType returns the session type for this instance.
func (i *Instance) GetSessionType() string {
	return i.SessionType
}

// IsDockerSession returns true if this instance uses a Docker session.
func (i *Instance) IsDockerSession() bool {
	return i.SessionType == config.SessionTypeDockerBind || i.SessionType == config.SessionTypeDockerClone
}

// CheckAndRestartProgram checks if the program needs to be restarted and does so if possible.
// This is used to handle system restarts where the Zellij session survives but the program
// (e.g., Claude) has exited. If a Claude session ID is available, it will restart with --resume.
func (i *Instance) CheckAndRestartProgram() error {
	if !i.started || i.Status == Paused {
		return nil
	}

	// Check if program is running
	running, err := i.session.IsProgramRunning()
	if err != nil {
		return fmt.Errorf("failed to check if program is running: %w", err)
	}

	if running {
		return nil // Program is running, nothing to do
	}

	// Program is not running, try to restart it
	log.InfoLog.Printf("Program not running in instance %s, attempting restart", i.Title)

	// For Claude, use --resume with session ID if available
	args := ""
	if strings.Contains(i.Program, "claude") && i.ClaudeSessionID != "" {
		args = "--resume " + i.ClaudeSessionID
		log.InfoLog.Printf("Restarting Claude with session ID: %s", i.ClaudeSessionID)
	}

	if err := i.session.RestartProgram(args); err != nil {
		return fmt.Errorf("failed to restart program: %w", err)
	}

	return nil
}

// CaptureClaudeSessionID captures and stores the Claude session ID from Claude's project files.
// This should be called after Claude has started and had time to create its session files.
// The session ID is used to resume the conversation after a system restart.
func (i *Instance) CaptureClaudeSessionID() {
	if !strings.Contains(i.Program, "claude") {
		return
	}

	// Get the worktree path for Claude's project directory
	worktreePath := ""
	if i.gitWorktree != nil {
		worktreePath = i.gitWorktree.GetWorktreePath()
	}
	if worktreePath == "" {
		worktreePath = i.Path
	}

	sessionID, err := ExtractClaudeSessionID(worktreePath)
	if err != nil {
		// Don't log warnings for expected errors (project not created yet, no session files yet)
		if !errors.Is(err, ErrClaudeProjectNotFound) && !errors.Is(err, ErrNoSessionFiles) {
			log.WarningLog.Printf("Failed to capture Claude session ID for %s: %v", i.Title, err)
		}
		return
	}

	if sessionID != "" && sessionID != i.ClaudeSessionID {
		i.ClaudeSessionID = sessionID
		log.InfoLog.Printf("Captured Claude session ID for %s: %s", i.Title, sessionID)
	}
}

// GetClaudeSessionID returns the stored Claude session ID.
func (i *Instance) GetClaudeSessionID() string {
	return i.ClaudeSessionID
}
