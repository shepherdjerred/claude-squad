package git

import (
	"claude-squad/log"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// ClaudeSettings represents the structure of .claude/settings.local.json
type ClaudeSettings struct {
	Permissions ClaudePermissions `json:"permissions"`
}

// ClaudePermissions represents the permissions section of Claude settings
type ClaudePermissions struct {
	Allow []string `json:"allow"`
}

// DefaultAllowedCommands are the commands that should be auto-approved in worktrees
var DefaultAllowedCommands = []string{
	"Bash(git:*)",
	"Bash(gh:*)",
}

// createClaudeSettingsFile creates a .claude/settings.local.json file in the worktree
// that auto-approves git and gh commands
func (g *GitWorktree) createClaudeSettingsFile() error {
	claudeDir := filepath.Join(g.worktreePath, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w", err)
	}

	settings := ClaudeSettings{
		Permissions: ClaudePermissions{
			Allow: DefaultAllowedCommands,
		},
	}

	settingsJSON, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	if err := os.WriteFile(settingsPath, settingsJSON, 0644); err != nil {
		return fmt.Errorf("failed to write settings file: %w", err)
	}

	return nil
}

// Setup creates a new worktree for the session
func (g *GitWorktree) Setup() error {
	g.reportProgress("Preparing worktree directory...")

	// Ensure worktrees directory exists early (can be done in parallel with branch check)
	worktreesDir, err := getWorktreeDirectory()
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	// Create directory and check branch existence in parallel
	errChan := make(chan error, 2)
	var branchExists bool

	// Goroutine for directory creation
	go func() {
		errChan <- os.MkdirAll(worktreesDir, 0755)
	}()

	// Goroutine for branch check
	go func() {
		repo, err := git.PlainOpen(g.repoPath)
		if err != nil {
			errChan <- fmt.Errorf("failed to open repository: %w", err)
			return
		}

		branchRef := plumbing.NewBranchReferenceName(g.branchName)
		if _, err := repo.Reference(branchRef, false); err == nil {
			branchExists = true
		}
		errChan <- nil
	}()

	// Wait for both operations
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			return err
		}
	}

	if branchExists {
		g.reportProgress(fmt.Sprintf("Setting up worktree from existing branch '%s'...", g.branchName))
		return g.setupFromExistingBranch()
	}
	g.reportProgress(fmt.Sprintf("Creating new worktree with branch '%s'...", g.branchName))
	return g.setupNewWorktree()
}

// setupFromExistingBranch creates a worktree from an existing branch
func (g *GitWorktree) setupFromExistingBranch() error {
	// Directory already created in Setup(), skip duplicate creation

	// Clean up any existing worktree first
	g.reportProgress("Cleaning up existing worktree...")
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath) // Ignore error if worktree doesn't exist

	// Create a new worktree from the existing branch
	g.reportProgress("Creating worktree...")
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", g.worktreePath, g.branchName); err != nil {
		return fmt.Errorf("failed to create worktree from branch %s: %w", g.branchName, err)
	}

	// Set the base commit SHA for diff computation
	// Find the merge-base between this branch and the default branch
	g.reportProgress("Computing base commit for diff...")
	if err := g.computeBaseCommitSHA(); err != nil {
		// Log the error but don't fail - diff stats just won't be available
		log.WarningLog.Printf("could not compute base commit SHA: %v", err)
	}

	// Create Claude settings file to auto-approve git/gh commands
	if err := g.createClaudeSettingsFile(); err != nil {
		log.WarningLog.Printf("failed to create Claude settings file: %v", err)
	}

	g.reportProgress("Worktree ready")
	return nil
}

// setupNewWorktree creates a new worktree from HEAD
func (g *GitWorktree) setupNewWorktree() error {
	// Ensure worktrees directory exists
	worktreesDir := filepath.Join(g.repoPath, "worktrees")
	if err := os.MkdirAll(worktreesDir, 0755); err != nil {
		return fmt.Errorf("failed to create worktrees directory: %w", err)
	}

	// Clean up any existing worktree first
	g.reportProgress("Cleaning up existing worktree...")
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath) // Ignore error if worktree doesn't exist

	// Open the repository
	g.reportProgress("Opening repository...")
	repo, err := git.PlainOpen(g.repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Clean up any existing branch or reference
	if err := g.cleanupExistingBranch(repo); err != nil {
		return fmt.Errorf("failed to cleanup existing branch: %w", err)
	}

	g.reportProgress("Getting HEAD commit...")
	output, err := g.runGitCommand(g.repoPath, "rev-parse", "HEAD")
	if err != nil {
		if strings.Contains(err.Error(), "fatal: ambiguous argument 'HEAD'") ||
			strings.Contains(err.Error(), "fatal: not a valid object name") ||
			strings.Contains(err.Error(), "fatal: HEAD: not a valid object name") {
			return fmt.Errorf("this appears to be a brand new repository: please create an initial commit before creating an instance")
		}
		return fmt.Errorf("failed to get HEAD commit hash: %w", err)
	}
	headCommit := strings.TrimSpace(string(output))
	g.baseCommitSHA = headCommit

	// Create a new worktree from the HEAD commit
	// Otherwise, we'll inherit uncommitted changes from the previous worktree.
	// This way, we can start the worktree with a clean slate.
	// TODO: we might want to give an option to use main/master instead of the current branch.
	g.reportProgress("Creating worktree...")
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", "-b", g.branchName, g.worktreePath, headCommit); err != nil {
		return fmt.Errorf("failed to create worktree from commit %s: %w", headCommit, err)
	}

	// Create Claude settings file to auto-approve git/gh commands
	if err := g.createClaudeSettingsFile(); err != nil {
		log.WarningLog.Printf("failed to create Claude settings file: %v", err)
	}

	g.reportProgress("Worktree ready")
	return nil
}

// Cleanup removes the worktree and associated branch
func (g *GitWorktree) Cleanup() error {
	var errs []error

	// Check if worktree path exists before attempting removal
	if _, err := os.Stat(g.worktreePath); err == nil {
		// Remove the worktree using git command
		if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath); err != nil {
			errs = append(errs, err)
		}
	} else if !os.IsNotExist(err) {
		// Only append error if it's not a "not exists" error
		errs = append(errs, fmt.Errorf("failed to check worktree path: %w", err))
	}

	// Open the repository for branch cleanup
	repo, err := git.PlainOpen(g.repoPath)
	if err != nil {
		// If the repository doesn't exist, there's nothing more to clean up
		// This can happen if the repo was deleted externally - this is the desired end state
		if err == git.ErrRepositoryNotExists || strings.Contains(err.Error(), "repository does not exist") {
			log.InfoLog.Printf("Repository %s does not exist, cleanup already complete", g.repoPath)
			return g.combineErrors(errs)
		}
		errs = append(errs, fmt.Errorf("failed to open repository for cleanup: %w", err))
		return g.combineErrors(errs)
	}

	branchRef := plumbing.NewBranchReferenceName(g.branchName)

	// Check if branch exists before attempting removal
	if _, err := repo.Reference(branchRef, false); err == nil {
		if err := repo.Storer.RemoveReference(branchRef); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove branch %s: %w", g.branchName, err))
		}
	} else if err != plumbing.ErrReferenceNotFound {
		errs = append(errs, fmt.Errorf("error checking branch %s existence: %w", g.branchName, err))
	}

	// Prune the worktree to clean up any remaining references
	if err := g.Prune(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return g.combineErrors(errs)
	}

	return nil
}

// Remove removes the worktree but keeps the branch
func (g *GitWorktree) Remove() error {
	// Remove the worktree using git command
	if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	return nil
}

// Prune removes all working tree administrative files and directories
func (g *GitWorktree) Prune() error {
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}
	return nil
}

// CleanupWorktrees removes all worktrees and their associated branches
func CleanupWorktrees() error {
	worktreesDir, err := getWorktreeDirectory()
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return fmt.Errorf("failed to read worktree directory: %w", err)
	}

	// Get a list of all branches associated with worktrees
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	// Parse the output to extract branch names
	worktreeBranches := make(map[string]string)
	currentWorktree := ""
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			currentWorktree = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			branchPath := strings.TrimPrefix(line, "branch ")
			// Extract branch name from refs/heads/branch-name
			branchName := strings.TrimPrefix(branchPath, "refs/heads/")
			if currentWorktree != "" {
				worktreeBranches[currentWorktree] = branchName
			}
		}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			worktreePath := filepath.Join(worktreesDir, entry.Name())

			// Delete the branch associated with this worktree if found
			for path, branch := range worktreeBranches {
				if strings.Contains(path, entry.Name()) {
					// Delete the branch
					deleteCmd := exec.Command("git", "branch", "-D", branch)
					if err := deleteCmd.Run(); err != nil {
						// Log the error but continue with other worktrees
						log.ErrorLog.Printf("failed to delete branch %s: %v", branch, err)
					}
					break
				}
			}

			// Remove the worktree directory
			os.RemoveAll(worktreePath)
		}
	}

	// You have to prune the cleaned up worktrees.
	cmd = exec.Command("git", "worktree", "prune")
	_, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}

	return nil
}

// computeBaseCommitSHA finds the merge-base between the current branch and the default branch
// This is used to compute diffs for existing branches that were resumed
func (g *GitWorktree) computeBaseCommitSHA() error {
	// Try to find the default branch (main, master, or remote HEAD)
	defaultBranch, err := g.findDefaultBranch()
	if err != nil {
		return fmt.Errorf("could not find default branch: %w", err)
	}

	// Find the merge-base between the current branch and the default branch
	mergeBase, err := g.runGitCommand(g.repoPath, "merge-base", g.branchName, defaultBranch)
	if err != nil {
		return fmt.Errorf("could not find merge-base: %w", err)
	}

	g.baseCommitSHA = strings.TrimSpace(mergeBase)
	return nil
}

// findDefaultBranch attempts to find the default branch of the repository
// It tries: remote HEAD, then main, then master
func (g *GitWorktree) findDefaultBranch() (string, error) {
	// Try to get the remote HEAD reference (most accurate for determining default branch)
	if output, err := g.runGitCommand(g.repoPath, "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
		// Output is like "refs/remotes/origin/main"
		ref := strings.TrimSpace(output)
		// Extract just the branch name (e.g., "main" from "refs/remotes/origin/main")
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}

	// Fallback: check if main exists
	if _, err := g.runGitCommand(g.repoPath, "rev-parse", "--verify", "main"); err == nil {
		return "main", nil
	}

	// Fallback: check if master exists
	if _, err := g.runGitCommand(g.repoPath, "rev-parse", "--verify", "master"); err == nil {
		return "master", nil
	}

	return "", fmt.Errorf("could not find default branch (tried origin/HEAD, main, master)")
}
