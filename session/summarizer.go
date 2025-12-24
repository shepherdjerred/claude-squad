package session

import (
	"claude-squad/log"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// SummaryRefreshInterval is how often to check for instances needing summary updates
	SummaryRefreshInterval = 5 * time.Second
	// SummaryPerInstanceCooldown is the minimum time between updates for a single instance
	SummaryPerInstanceCooldown = 10 * time.Second
	// SummaryMaxLength is the maximum length of a summary
	SummaryMaxLength = 80
)

// Patterns for extracting information from terminal content
var (
	// Match file paths with common extensions
	filePathPattern = regexp.MustCompile(`[\w./\-]+\.(go|ts|tsx|js|jsx|py|rs|java|c|cpp|h|hpp|md|json|yaml|yml|toml|html|css|scss|sql|sh|bash|zsh)`)
	// Match tool actions from Claude Code output
	toolActionPattern = regexp.MustCompile(`(?i)(Read|Edit|Write|Bash|Grep|Glob|Task|WebFetch|WebSearch|TodoWrite)\s*\(`)
	// Match "Thinking..." or similar status indicators
	thinkingPattern = regexp.MustCompile(`(?i)(thinking|processing|analyzing|working)\.{0,3}`)
	// Match waiting for input patterns
	waitingPattern = regexp.MustCompile(`(?i)(waiting|ready|idle|>|\$|%)`)
	// Match error/warning indicators
	errorPattern = regexp.MustCompile(`(?i)(error|failed|warning|exception|panic)`)
	// Match success indicators
	successPattern = regexp.MustCompile(`(?i)(success|completed|done|passed|✓|✔)`)
	// Match test-related output
	testPattern = regexp.MustCompile(`(?i)(test|spec|PASS|FAIL|ok\s+\d|---\s*(PASS|FAIL))`)
	// Match build/compile related output
	buildPattern = regexp.MustCompile(`(?i)(build|compile|bundl|webpack|vite|esbuild)`)
	// Match git operations
	gitPattern = regexp.MustCompile(`(?i)(commit|push|pull|merge|rebase|checkout|branch)`)
)

// Summarizer handles generating AI-powered summaries for instances
type Summarizer struct {
	mu sync.Mutex
	// lastUpdateIndex tracks which instance was last updated for staggered refresh
	lastUpdateIndex int
}

// NewSummarizer creates a new Summarizer
func NewSummarizer() *Summarizer {
	return &Summarizer{}
}

// UpdateNextSummary updates the summary for the next instance in the rotation
// Returns the instance that was updated, or nil if no update was performed
// Each instance is only updated at most once per SummaryPerInstanceCooldown
func (s *Summarizer) UpdateNextSummary(instances []*Instance) *Instance {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(instances) == 0 {
		return nil
	}

	now := time.Now()

	// Find the next eligible instance (not paused, started, and not recently updated)
	startIdx := s.lastUpdateIndex
	for i := 0; i < len(instances); i++ {
		idx := (startIdx + i) % len(instances)
		instance := instances[idx]

		// Skip paused or not-started instances
		if !instance.Started() || instance.Paused() {
			continue
		}

		// Skip if this instance was updated within the cooldown period
		if !instance.SummaryUpdatedAt.IsZero() && now.Sub(instance.SummaryUpdatedAt) < SummaryPerInstanceCooldown {
			continue
		}

		// Update the index for next time
		s.lastUpdateIndex = (idx + 1) % len(instances)

		// Generate summary for this instance
		if err := s.generateSummary(instance); err != nil {
			log.WarningLog.Printf("Failed to generate summary for %s: %v", instance.Title, err)
			return nil
		}

		return instance
	}

	return nil
}

// generateSummary generates a summary for the given instance by parsing terminal content
func (s *Summarizer) generateSummary(instance *Instance) error {
	// Get the current terminal content
	content, err := instance.Preview()
	if err != nil {
		return err
	}

	if content == "" {
		instance.Summary = "No output yet"
		instance.SummaryUpdatedAt = time.Now()
		return nil
	}

	// Extract summary from terminal content
	summary := extractSummaryFromContent(content)

	instance.Summary = summary
	instance.SummaryUpdatedAt = time.Now()

	return nil
}

// extractSummaryFromContent parses terminal content and extracts a meaningful summary
func extractSummaryFromContent(content string) string {
	// Focus on the last portion of content (most recent activity)
	lines := strings.Split(content, "\n")

	// Get last 30 lines for analysis
	startIdx := 0
	if len(lines) > 30 {
		startIdx = len(lines) - 30
	}
	recentLines := lines[startIdx:]
	recentContent := strings.Join(recentLines, "\n")

	var parts []string

	// Check for specific states/activities in priority order

	// 1. Check for errors first (high priority)
	if errorPattern.MatchString(recentContent) {
		parts = append(parts, "Error detected")
	}

	// 2. Check for thinking/processing state
	if thinkingPattern.MatchString(recentContent) {
		parts = append(parts, "Processing")
	}

	// 3. Check for tool actions
	if matches := toolActionPattern.FindStringSubmatch(recentContent); len(matches) > 1 {
		toolName := matches[1]
		parts = append(parts, toolName)
	}

	// 4. Check for specific activity types
	if testPattern.MatchString(recentContent) {
		if successPattern.MatchString(recentContent) {
			parts = append(parts, "Tests passing")
		} else if errorPattern.MatchString(recentContent) {
			parts = append(parts, "Tests failing")
		} else {
			parts = append(parts, "Running tests")
		}
	} else if buildPattern.MatchString(recentContent) {
		parts = append(parts, "Building")
	} else if gitPattern.MatchString(recentContent) {
		parts = append(parts, "Git operations")
	}

	// 5. Extract file paths being worked on
	if matches := filePathPattern.FindAllString(recentContent, 3); len(matches) > 0 {
		// Get just the filename, not the full path
		for i, match := range matches {
			pathParts := strings.Split(match, "/")
			matches[i] = pathParts[len(pathParts)-1]
		}
		// Dedupe
		seen := make(map[string]bool)
		var unique []string
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				unique = append(unique, m)
			}
		}
		if len(unique) > 0 {
			if len(unique) > 2 {
				unique = unique[:2]
			}
			parts = append(parts, strings.Join(unique, ", "))
		}
	}

	// 6. Check for success/completion
	if successPattern.MatchString(recentContent) && !testPattern.MatchString(recentContent) {
		parts = append(parts, "Completed")
	}

	// 7. Check for waiting/idle state (low priority)
	if len(parts) == 0 && waitingPattern.MatchString(recentContent) {
		parts = append(parts, "Waiting for input")
	}

	// Build final summary
	if len(parts) == 0 {
		return "Active"
	}

	summary := strings.Join(parts, " - ")
	if len(summary) > SummaryMaxLength {
		summary = summary[:SummaryMaxLength-3] + "..."
	}

	return summary
}

// GetSummary returns the summary for an instance, or a placeholder if none exists
func GetSummary(instance *Instance) string {
	if instance == nil {
		return ""
	}
	if instance.Summary == "" {
		return ""
	}
	return instance.Summary
}
