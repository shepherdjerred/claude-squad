package git

import (
	"strings"
	"time"
)

// Default cache duration for diff stats
const defaultDiffCacheDuration = 5 * time.Second

// DiffStats holds statistics about the changes in a diff
type DiffStats struct {
	// Content is the full diff content
	Content string
	// Added is the number of added lines
	Added int
	// Removed is the number of removed lines
	Removed int
	// Error holds any error that occurred during diff computation
	// This allows propagating setup errors (like missing base commit) without breaking the flow
	Error error
}

func (d *DiffStats) IsEmpty() bool {
	return d.Added == 0 && d.Removed == 0 && d.Content == ""
}

// isDirty performs a quick check to see if the worktree has uncommitted changes.
// This is much faster than running a full diff.
func (g *GitWorktree) isDirty() (bool, error) {
	output, err := g.runGitCommand(g.worktreePath, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(output)) > 0, nil
}

// Diff returns the git diff between the worktree and the base branch along with statistics.
// Results are cached for up to 5 seconds to reduce expensive git operations.
func (g *GitWorktree) Diff() *DiffStats {
	// Initialize cache duration if not set
	if g.diffCacheDuration == 0 {
		g.diffCacheDuration = defaultDiffCacheDuration
	}

	// Check if we have a valid cached result
	if g.cachedDiffStats != nil && time.Since(g.diffCacheTime) < g.diffCacheDuration {
		// Quick dirty check - if no changes, return cached empty stats
		if g.cachedDiffStats.IsEmpty() {
			dirty, err := g.isDirty()
			if err == nil && !dirty {
				return g.cachedDiffStats
			}
		} else {
			// Cache is still valid, return it
			return g.cachedDiffStats
		}
	}

	// Run the full diff
	stats := g.diffUncached()

	// Cache the result
	g.cachedDiffStats = stats
	g.diffCacheTime = time.Now()

	return stats
}

// diffUncached performs the actual git diff operation without caching
func (g *GitWorktree) diffUncached() *DiffStats {
	stats := &DiffStats{}

	// -N stages untracked files (intent to add), including them in the diff
	_, err := g.runGitCommand(g.worktreePath, "add", "-N", ".")
	if err != nil {
		stats.Error = err
		return stats
	}

	content, err := g.runGitCommand(g.worktreePath, "--no-pager", "diff", g.GetBaseCommitSHA())
	if err != nil {
		stats.Error = err
		return stats
	}
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			stats.Added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			stats.Removed++
		}
	}
	stats.Content = content

	return stats
}

// InvalidateDiffCache clears the cached diff stats, forcing the next Diff() call
// to perform a fresh git diff operation. Call this when you know the worktree
// has changed (e.g., after Resume).
func (g *GitWorktree) InvalidateDiffCache() {
	g.cachedDiffStats = nil
	g.diffCacheTime = time.Time{}
}
