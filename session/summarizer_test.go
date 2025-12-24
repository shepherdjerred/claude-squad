package session

import (
	"testing"
)

func TestExtractSummaryFromContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "empty content",
			content:  "",
			expected: "Active",
		},
		{
			name:     "file path detection",
			content:  "Reading /path/to/file.go\nSome content here",
			expected: "file.go",
		},
		{
			name:     "multiple file paths",
			content:  "Editing main.go and utils.ts",
			expected: "main.go, utils.ts",
		},
		{
			name:     "tool action detection",
			content:  "Calling Edit(file.go)",
			expected: "Edit - file.go",
		},
		{
			name:     "error detection",
			content:  "Error: failed to compile\nsome other output",
			expected: "Error detected",
		},
		{
			name:     "test passing",
			content:  "PASS: TestSomething\nok test completed",
			expected: "Tests passing",
		},
		{
			name:     "test failing",
			content:  "FAIL: TestSomething\nerror in test",
			expected: "Error detected - Tests failing",
		},
		{
			name:     "git operations",
			content:  "git commit -m 'fix bug'\nsome output",
			expected: "Git operations",
		},
		{
			name:     "build detection",
			content:  "Building project...\nwebpack running",
			expected: "Building",
		},
		{
			name:     "thinking/processing",
			content:  "Thinking...\nAnalyzing code",
			expected: "Processing",
		},
		{
			name:     "waiting for input",
			content:  ">\nReady for input",
			expected: "Waiting for input",
		},
		{
			name:     "success completion",
			content:  "Task completed successfully",
			expected: "Completed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSummaryFromContent(tt.content)
			// Check if the expected string is contained in the result
			// (since multiple patterns may match)
			if result != tt.expected && !containsExpected(result, tt.expected) {
				t.Errorf("extractSummaryFromContent(%q) = %q, want %q", tt.content, result, tt.expected)
			}
		})
	}
}

func containsExpected(result, expected string) bool {
	// For cases where multiple patterns match, check if expected is contained
	return len(result) > 0 && (result == expected ||
		(len(expected) > 0 && len(result) >= len(expected) &&
			(result[:len(expected)] == expected || result[len(result)-len(expected):] == expected)))
}

func TestExtractSummaryMaxLength(t *testing.T) {
	// Create content that would generate a very long summary
	longContent := "file1.go file2.go file3.go file4.go file5.go " +
		"Error detected " +
		"Thinking... " +
		"Building project"

	result := extractSummaryFromContent(longContent)

	if len(result) > SummaryMaxLength {
		t.Errorf("summary length %d exceeds max length %d", len(result), SummaryMaxLength)
	}
}
