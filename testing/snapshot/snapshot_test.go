package snapshot

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no ansi codes",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "simple color code",
			input:    "\x1b[31mred\x1b[0m",
			expected: "red",
		},
		{
			name:     "multiple codes",
			input:    "\x1b[1;31mbold red\x1b[0m and \x1b[32mgreen\x1b[0m",
			expected: "bold red and green",
		},
		{
			name:     "osc8 hyperlink",
			input:    "\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\",
			expected: "link",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripANSI(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "single line",
			input:    "hello",
			expected: 1,
		},
		{
			name:     "multiple lines",
			input:    "line1\nline2\nline3",
			expected: 3,
		},
		{
			name:     "with ansi codes",
			input:    "\x1b[31mred\x1b[0m\nblue",
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Lines(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWidth(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "single line",
			input:    "hello",
			expected: 5,
		},
		{
			name:     "multiple lines varying width",
			input:    "short\nlonger line\nmed",
			expected: 11, // "longer line" is 11 chars
		},
		{
			name:     "with ansi codes",
			input:    "\x1b[31mhello world\x1b[0m",
			expected: 11, // "hello world" without codes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Width(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeOutput(t *testing.T) {
	input := "line with trailing spaces   \n\x1b[31mcolored\x1b[0m\r\n"
	result := normalizeOutput(input)

	// Should strip ANSI, normalize line endings, remove trailing spaces
	assert.NotContains(t, result, "\x1b")
	assert.NotContains(t, result, "\r")
	assert.NotContains(t, result, "   \n")
}
