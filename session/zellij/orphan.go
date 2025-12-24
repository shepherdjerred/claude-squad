package zellij

import (
	"claude-squad/cmd"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// OrphanedSession represents a Zellij session not tracked in state.json
type OrphanedSession struct {
	SessionName  string // Full session name (e.g., "claudesquad_MyTask")
	Title        string // Extracted title (e.g., "MyTask")
	WorktreePath string // from dump-layout cwd
	Program      string // from dump-layout command
	BranchName   string // extracted from worktree path
	RepoPath     string // from git worktree list
}

// ListOrphanedSessions returns active claudesquad_ sessions not in state.json
// trackedTitles should be the titles of instances currently in state.json
func ListOrphanedSessions(trackedTitles []string, cmdExec cmd.Executor) ([]OrphanedSession, error) {
	if cmdExec == nil {
		cmdExec = cmd.MakeExecutor()
	}

	// Run zellij list-sessions
	listCmd := exec.Command("zellij", "list-sessions")
	output, err := cmdExec.Output(listCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list zellij sessions: %w", err)
	}

	// Strip ANSI escape codes
	cleanOutput := ansiEscapeRegex.ReplaceAllString(string(output), "")

	// Create a set of tracked titles for quick lookup
	trackedSet := make(map[string]bool)
	for _, title := range trackedTitles {
		trackedSet[title] = true
	}

	var orphans []OrphanedSession
	lines := strings.Split(cleanOutput, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip EXITED sessions
		if strings.Contains(line, "EXITED") {
			continue
		}

		// Extract session name (first field)
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		sessionName := fields[0]

		// Only process claudesquad_ prefixed sessions
		if !strings.HasPrefix(sessionName, ZellijPrefix) {
			continue
		}

		// Extract title by removing prefix
		title := strings.TrimPrefix(sessionName, ZellijPrefix)

		// Skip if this session is already tracked
		if trackedSet[title] {
			continue
		}

		orphans = append(orphans, OrphanedSession{
			SessionName: sessionName,
			Title:       title,
		})
	}

	return orphans, nil
}

// RecoverMetadata uses dump-layout to recover session metadata
func RecoverMetadata(sessionName string, cmdExec cmd.Executor) (*OrphanedSession, error) {
	if cmdExec == nil {
		cmdExec = cmd.MakeExecutor()
	}

	// Run zellij dump-layout
	dumpCmd := exec.Command("zellij", "-s", sessionName, "action", "dump-layout")
	output, err := cmdExec.Output(dumpCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to dump layout for session %s: %w", sessionName, err)
	}

	layoutContent := string(output)

	// Extract cwd from layout (pattern: cwd "path")
	cwd := extractKDLValue(layoutContent, "cwd")

	// Extract command from layout (pattern: command "cmd")
	program := extractKDLValue(layoutContent, "command")

	// Extract title from session name
	title := strings.TrimPrefix(sessionName, ZellijPrefix)

	orphan := &OrphanedSession{
		SessionName:  sessionName,
		Title:        title,
		WorktreePath: cwd,
		Program:      program,
	}

	// Try to extract branch name from worktree path
	// Pattern: ~/.claude-squad/worktrees/{branch}_{timestamp}
	if cwd != "" && strings.Contains(cwd, ".claude-squad/worktrees/") {
		orphan.BranchName = extractBranchFromWorktreePath(cwd)
		// Try to get repo path from git worktree list
		orphan.RepoPath = getRepoPathFromWorktree(cwd, cmdExec)
	}

	return orphan, nil
}

// extractKDLValue extracts the value for a given key from KDL content
// Example: cwd "/path/to/dir" -> "/path/to/dir"
func extractKDLValue(content, key string) string {
	// Match pattern: key "value" or key value
	pattern := regexp.MustCompile(key + `\s+"([^"]+)"`)
	matches := pattern.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// extractBranchFromWorktreePath extracts the branch name from a worktree path
// Pattern: ~/.claude-squad/worktrees/{branch}_{timestamp}
func extractBranchFromWorktreePath(worktreePath string) string {
	// Get the base name of the path
	baseName := filepath.Base(worktreePath)

	// Find the last underscore followed by hex characters (the timestamp)
	// Pattern: branch_name_18840cf732e4c550
	pattern := regexp.MustCompile(`^(.+)_[0-9a-f]{16}$`)
	matches := pattern.FindStringSubmatch(baseName)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// getRepoPathFromWorktree tries to get the original repo path from a git worktree
func getRepoPathFromWorktree(worktreePath string, cmdExec cmd.Executor) string {
	// Run git worktree list --porcelain from the worktree directory
	listCmd := exec.Command("git", "-C", worktreePath, "worktree", "list", "--porcelain")
	output, err := cmdExec.Output(listCmd)
	if err != nil {
		return ""
	}

	// Parse output to find the main worktree (the one that's not marked as a linked worktree)
	// Format:
	// worktree /path/to/main/repo
	// HEAD abc123
	// branch refs/heads/main
	//
	// worktree /path/to/linked/worktree
	// HEAD def456
	// branch refs/heads/feature
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			path := strings.TrimPrefix(line, "worktree ")
			// The first worktree in the list is typically the main repo
			// Skip if it's the same as our worktree path (which is a linked worktree)
			if path != worktreePath && !strings.Contains(path, ".claude-squad/worktrees/") {
				return path
			}
		}
	}

	return ""
}
