package ui

import (
	"fmt"
	"time"
)

// FormatRelativeTime formats a time as a human-readable relative string.
// Examples: "just now", "2m ago", "3h ago", "5d ago", "2mo ago", "1y ago"
func FormatRelativeTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		return fmt.Sprintf("%dm ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		return fmt.Sprintf("%dh ago", hours)
	case diff < 30*24*time.Hour:
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / (24 * 30))
		return fmt.Sprintf("%dmo ago", months)
	default:
		years := int(diff.Hours() / (24 * 365))
		return fmt.Sprintf("%dy ago", years)
	}
}

// FormatLastOpened formats the last opened time, handling nil (never opened).
func FormatLastOpened(t *time.Time) string {
	if t == nil {
		return "never"
	}
	return FormatRelativeTime(*t)
}
