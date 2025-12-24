package overlay

import (
	"claude-squad/config"
	"claude-squad/session"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ModeOption represents a selectable session mode option
type ModeOption struct {
	Type        string // config.SessionType constant
	Name        string // Display name
	Description string // Description of when to use
	Available   bool   // Whether this option is available (e.g., Docker installed)
}

// ModeSelectorOverlay represents a session mode selection dialog
type ModeSelectorOverlay struct {
	Dismissed bool
	Selected  string // The selected session type
	options   []ModeOption
	cursor    int
	width     int
}

// NewModeSelectorOverlay creates a new mode selector overlay
func NewModeSelectorOverlay() *ModeSelectorOverlay {
	dockerAvailable := session.IsDockerAvailable()

	options := []ModeOption{
		{
			Type:        config.SessionTypeZellij,
			Name:        "Zellij (local terminal)",
			Description: "Run Claude in a Zellij terminal session on your machine.\nBest for: Quick tasks, when you want direct file access.",
			Available:   session.IsZellijAvailable(),
		},
		{
			Type:        config.SessionTypeDockerBind,
			Name:        "Docker (bind-mount)",
			Description: "Run Claude in a container with your code mounted.\nBest for: Consistent environment, isolated dependencies.",
			Available:   dockerAvailable,
		},
		{
			Type:        config.SessionTypeDockerClone,
			Name:        "Docker (clone)",
			Description: "Clone repo inside container (fully isolated).\nBest for: Untrusted code, sandboxed experiments.",
			Available:   dockerAvailable,
		},
	}

	return &ModeSelectorOverlay{
		options: options,
		cursor:  0,
		width:   60,
	}
}

// HandleKeyPress processes a key press and updates the state
func (m *ModeSelectorOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "up", "k":
		m.moveCursor(-1)
		return false
	case "down", "j":
		m.moveCursor(1)
		return false
	case "enter":
		if m.options[m.cursor].Available {
			m.Selected = m.options[m.cursor].Type
			m.Dismissed = true
			return true
		}
		return false
	case "esc":
		m.Dismissed = true
		return true
	default:
		return false
	}
}

// moveCursor moves the cursor up or down, skipping unavailable options
func (m *ModeSelectorOverlay) moveCursor(delta int) {
	newCursor := m.cursor + delta

	// Wrap around
	if newCursor < 0 {
		newCursor = len(m.options) - 1
	} else if newCursor >= len(m.options) {
		newCursor = 0
	}

	// Skip unavailable options
	for attempts := 0; attempts < len(m.options); attempts++ {
		if m.options[newCursor].Available {
			m.cursor = newCursor
			return
		}
		newCursor += delta
		if newCursor < 0 {
			newCursor = len(m.options) - 1
		} else if newCursor >= len(m.options) {
			newCursor = 0
		}
	}
}

// Render renders the mode selector overlay
func (m *ModeSelectorOverlay) Render(opts ...WhitespaceOption) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF"))

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7aa2f7")).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#AAAAAA"))

	unavailableStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#555555")).
		Strikethrough(true)

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		PaddingLeft(4)

	unavailableDescStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#444444")).
		PaddingLeft(4)

	var content strings.Builder
	content.WriteString(titleStyle.Render("Select Session Mode"))
	content.WriteString("\n\n")

	for i, opt := range m.options {
		var prefix string
		var nameStyle, descStyleToUse lipgloss.Style

		if i == m.cursor && opt.Available {
			prefix = "> "
			nameStyle = selectedStyle
			descStyleToUse = descStyle
		} else if !opt.Available {
			prefix = "  "
			nameStyle = unavailableStyle
			descStyleToUse = unavailableDescStyle
		} else {
			prefix = "  "
			nameStyle = normalStyle
			descStyleToUse = descStyle
		}

		content.WriteString(prefix)
		content.WriteString(nameStyle.Render(opt.Name))
		if !opt.Available {
			content.WriteString(" (not installed)")
		}
		content.WriteString("\n")

		// Add description
		for _, line := range strings.Split(opt.Description, "\n") {
			content.WriteString(descStyleToUse.Render(line))
			content.WriteString("\n")
		}
		content.WriteString("\n")
	}

	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(
		"[Enter] Select  [Esc] Cancel  [↑/↓] Navigate"))

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7aa2f7")).
		Padding(1, 2).
		Width(m.width)

	return borderStyle.Render(content.String())
}

// SetWidth sets the width of the overlay
func (m *ModeSelectorOverlay) SetWidth(width int) {
	m.width = width
}

// GetSelected returns the selected session type
func (m *ModeSelectorOverlay) GetSelected() string {
	return m.Selected
}
