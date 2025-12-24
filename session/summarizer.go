package session

import (
	"bytes"
	"claude-squad/log"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// SummaryRefreshInterval is how often to check for instances needing summary updates
	SummaryRefreshInterval = 5 * time.Second
	// SummaryPerInstanceCooldown is the minimum time between updates for a single instance
	SummaryPerInstanceCooldown = 60 * time.Second
	// SummaryMaxLength is the maximum length of a summary
	SummaryMaxLength = 80
	// SummaryTimeout is the timeout for generating a summary
	SummaryTimeout = 30 * time.Second
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

// generateSummary generates a summary for the given instance using Claude CLI
func (s *Summarizer) generateSummary(instance *Instance) error {
	// Get the current terminal content
	content, err := instance.Preview()
	if err != nil {
		return fmt.Errorf("failed to get preview: %w", err)
	}

	if content == "" {
		instance.Summary = "No output yet"
		instance.SummaryUpdatedAt = time.Now()
		return nil
	}

	// Truncate content if it's too long (keep last part which is more relevant)
	const maxContentLen = 4000
	if len(content) > maxContentLen {
		content = content[len(content)-maxContentLen:]
	}

	// Build the prompt
	prompt := fmt.Sprintf(`Summarize what's happening in this Claude Code terminal session in 10 words or less. Focus on the current action or state. Be concise. Only output the summary, nothing else.

Terminal output:
%s`, content)

	// Call Claude CLI with --print flag for non-interactive mode
	ctx, cancel := context.WithTimeout(context.Background(), SummaryTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "--print", prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("summary generation timed out")
		}
		return fmt.Errorf("claude command failed: %w (stderr: %s)", err, stderr.String())
	}

	// Clean up the summary
	summary := strings.TrimSpace(stdout.String())
	if len(summary) > SummaryMaxLength {
		summary = summary[:SummaryMaxLength-3] + "..."
	}

	// Remove any quotes if Claude wrapped the response
	summary = strings.Trim(summary, "\"'")

	instance.Summary = summary
	instance.SummaryUpdatedAt = time.Now()

	return nil
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
