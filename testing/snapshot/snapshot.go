// Package snapshot provides golden file testing for TUI components.
// It captures rendered output and compares it against known-good files.
package snapshot

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// GoldenDir is the default directory for golden files
const GoldenDir = "testdata/golden"

// Snap provides snapshot testing functionality
type Snap struct {
	t         *testing.T
	goldenDir string
	update    bool
}

// New creates a new Snap instance for the given test
func New(t *testing.T) *Snap {
	return &Snap{
		t:         t,
		goldenDir: GoldenDir,
		update:    os.Getenv("UPDATE_GOLDEN") == "1",
	}
}

// WithDir sets a custom golden file directory
func (s *Snap) WithDir(dir string) *Snap {
	s.goldenDir = dir
	return s
}

// Assert compares actual output against a golden file.
// If UPDATE_GOLDEN=1, updates the golden file instead.
func (s *Snap) Assert(name, actual string) {
	s.t.Helper()

	goldenPath := filepath.Join(s.goldenDir, name+".golden")

	// Normalize the actual output
	normalized := normalizeOutput(actual)

	if s.update {
		// Update mode: write the new golden file
		if err := os.MkdirAll(s.goldenDir, 0755); err != nil {
			s.t.Fatalf("failed to create golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(normalized), 0644); err != nil {
			s.t.Fatalf("failed to write golden file: %v", err)
		}
		s.t.Logf("Updated golden file: %s", goldenPath)
		return
	}

	// Compare mode: read golden and compare
	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.t.Fatalf("Golden file not found: %s\nRun with UPDATE_GOLDEN=1 to create it.\nActual output:\n%s", goldenPath, normalized)
		}
		s.t.Fatalf("failed to read golden file: %v", err)
	}

	if string(expected) != normalized {
		s.t.Errorf("Snapshot mismatch for %s\n\nExpected:\n%s\n\nActual:\n%s\n\nRun with UPDATE_GOLDEN=1 to update.",
			name, string(expected), normalized)
	}
}

// AssertContains checks that actual output contains the expected substring
func (s *Snap) AssertContains(actual, substr string) {
	s.t.Helper()
	normalized := normalizeOutput(actual)
	if !strings.Contains(normalized, substr) {
		s.t.Errorf("Output does not contain expected substring.\nExpected to contain: %q\nActual:\n%s", substr, normalized)
	}
}

// AssertNotContains checks that actual output does NOT contain the substring
func (s *Snap) AssertNotContains(actual, substr string) {
	s.t.Helper()
	normalized := normalizeOutput(actual)
	if strings.Contains(normalized, substr) {
		s.t.Errorf("Output unexpectedly contains substring: %q\nActual:\n%s", substr, normalized)
	}
}

// normalizeOutput strips ANSI codes and normalizes whitespace for comparison
func normalizeOutput(s string) string {
	// Strip ANSI escape codes
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	s = ansiRegex.ReplaceAllString(s, "")

	// Strip OSC 8 hyperlink sequences
	oscRegex := regexp.MustCompile(`\x1b\]8;;[^\x1b]*\x1b\\`)
	s = oscRegex.ReplaceAllString(s, "")

	// Normalize line endings
	s = strings.ReplaceAll(s, "\r\n", "\n")

	// Remove trailing whitespace from each line
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}

	return strings.Join(lines, "\n")
}

// StripANSI removes all ANSI escape codes from a string
func StripANSI(s string) string {
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	s = ansiRegex.ReplaceAllString(s, "")

	// Also strip OSC 8 hyperlink sequences
	oscRegex := regexp.MustCompile(`\x1b\]8;;[^\x1b]*\x1b\\`)
	return oscRegex.ReplaceAllString(s, "")
}

// Lines returns the line count of the rendered output (useful for height tests)
func Lines(s string) int {
	return len(strings.Split(StripANSI(s), "\n"))
}

// Width returns the maximum line width of the rendered output
func Width(s string) int {
	stripped := StripANSI(s)
	lines := strings.Split(stripped, "\n")
	maxWidth := 0
	for _, line := range lines {
		if len(line) > maxWidth {
			maxWidth = len(line)
		}
	}
	return maxWidth
}
