package git

import (
	"claude-squad/config"
	"claude-squad/log"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func getWorktreeDirectory() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "worktrees"), nil
}

// ProgressCallback is called with status messages during setup
type ProgressCallback func(message string)

// GitWorktree manages git worktree operations for a session
type GitWorktree struct {
	// Path to the repository
	repoPath string
	// Path to the worktree
	worktreePath string
	// Name of the session
	sessionName string
	// Branch name for the worktree
	branchName string
	// Base commit hash for the worktree
	baseCommitSHA string
	// Progress callback for status updates
	progressCallback ProgressCallback

	// Diff caching
	cachedDiffStats   *DiffStats
	diffCacheTime     time.Time
	diffCacheDuration time.Duration
}

func NewGitWorktreeFromStorage(repoPath string, worktreePath string, sessionName string, branchName string, baseCommitSHA string) *GitWorktree {
	return &GitWorktree{
		repoPath:      repoPath,
		worktreePath:  worktreePath,
		sessionName:   sessionName,
		branchName:    branchName,
		baseCommitSHA: baseCommitSHA,
	}
}

// NewGitWorktree creates a new GitWorktree instance
func NewGitWorktree(repoPath string, sessionName string) (tree *GitWorktree, branchname string, err error) {
	cfg := config.LoadConfig()
	branchName := fmt.Sprintf("%s%s", cfg.BranchPrefix, sessionName)
	// Sanitize the final branch name to handle invalid characters from any source
	// (e.g., backslashes from Windows domain usernames like DOMAIN\user)
	branchName = sanitizeBranchName(branchName)

	// Convert repoPath to absolute path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		log.ErrorLog.Printf("git worktree path abs error, falling back to repoPath %s: %s", repoPath, err)
		// If we can't get absolute path, use original path as fallback
		absPath = repoPath
	}

	repoPath, err = findGitRepoRoot(absPath)
	if err != nil {
		return nil, "", err
	}

	worktreeDir, err := getWorktreeDirectory()
	if err != nil {
		return nil, "", err
	}

	// Use sanitized branch name for the worktree directory name
	worktreePath := filepath.Join(worktreeDir, branchName)
	// Extract suffix from session name for worktree path uniqueness
	suffix := extractSuffixFromSessionName(sessionName)
	if suffix != "" {
		worktreePath = worktreePath + "_" + suffix
	} else {
		// Fallback to timestamp for backward compatibility
		worktreePath = worktreePath + "_" + fmt.Sprintf("%x", time.Now().UnixNano())
	}

	return &GitWorktree{
		repoPath:     repoPath,
		sessionName:  sessionName,
		branchName:   branchName,
		worktreePath: worktreePath,
	}, branchName, nil
}

// GetWorktreePath returns the path to the worktree
func (g *GitWorktree) GetWorktreePath() string {
	return g.worktreePath
}

// GetBranchName returns the name of the branch associated with this worktree
func (g *GitWorktree) GetBranchName() string {
	return g.branchName
}

// GetRepoPath returns the path to the repository
func (g *GitWorktree) GetRepoPath() string {
	return g.repoPath
}

// GetRepoName returns the name of the repository (last part of the repoPath).
func (g *GitWorktree) GetRepoName() string {
	return filepath.Base(g.repoPath)
}

// GetBaseCommitSHA returns the base commit SHA for the worktree
func (g *GitWorktree) GetBaseCommitSHA() string {
	return g.baseCommitSHA
}

// GetSessionName returns the session name for this worktree.
// This is the original name used to create the multiplexer session and should
// not change when the instance is renamed.
func (g *GitWorktree) GetSessionName() string {
	return g.sessionName
}

// SetProgressCallback sets the callback function for progress updates
func (g *GitWorktree) SetProgressCallback(callback ProgressCallback) {
	g.progressCallback = callback
}

// reportProgress safely calls the progress callback if set
func (g *GitWorktree) reportProgress(message string) {
	if g.progressCallback != nil {
		g.progressCallback(message)
	}
}

// extractSuffixFromSessionName extracts the random word suffix from a session name.
// Returns empty string if no suffix is found.
func extractSuffixFromSessionName(sessionName string) string {
	parts := strings.Split(sessionName, "_")
	if len(parts) >= 2 {
		// Check if last part looks like a word suffix (lowercase letters only)
		lastPart := parts[len(parts)-1]
		if isWordSuffix(lastPart) {
			return lastPart
		}
	}
	return ""
}

// isWordSuffix validates if a string looks like a random word suffix
func isWordSuffix(s string) bool {
	// Simple validation: only lowercase letters, 3-15 chars
	if len(s) < 3 || len(s) > 15 {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}
