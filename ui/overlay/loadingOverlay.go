package overlay

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// LoadingOverlay represents a loading screen overlay with a spinner and status message
type LoadingOverlay struct {
	// Title displayed at the top
	title string
	// Current status message
	status string
	// Spinner for the loading animation
	spinner *spinner.Model

	width int
}

// NewLoadingOverlay creates a new loading screen overlay
func NewLoadingOverlay(title string, spinner *spinner.Model) *LoadingOverlay {
	return &LoadingOverlay{
		title:   title,
		status:  "",
		spinner: spinner,
	}
}

// SetStatus updates the current status message
func (l *LoadingOverlay) SetStatus(status string) {
	l.status = status
}

// SetWidth sets the overlay width
func (l *LoadingOverlay) SetWidth(width int) {
	l.width = width
}

// Render renders the loading overlay
func (l *LoadingOverlay) Render(opts ...WhitespaceOption) string {
	// Create styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62"))

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(l.width)

	// Build content
	content := titleStyle.Render(l.title) + "\n\n"
	if l.spinner != nil {
		content += l.spinner.View() + " "
	}
	content += statusStyle.Render(l.status)

	return boxStyle.Render(content)
}
