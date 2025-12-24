package ui

import "github.com/charmbracelet/lipgloss"

// Semantic Color Palette
// Designed for accessibility (colorblind-safe) with both color and shape differentiation.

// Status colors - each status has a distinct color and associated icon
var (
	// StatusSuccess indicates ready/complete state
	// Color: Green, Icon: "+"
	StatusSuccess = lipgloss.AdaptiveColor{Light: "#22C55E", Dark: "#22C55E"}

	// StatusRunning indicates in-progress state
	// Color: Blue, Icon: spinner
	StatusRunning = lipgloss.AdaptiveColor{Light: "#3B82F6", Dark: "#3B82F6"}

	// StatusWarning indicates needs attention
	// Color: Amber, Icon: "!"
	StatusWarning = lipgloss.AdaptiveColor{Light: "#F59E0B", Dark: "#F59E0B"}

	// StatusError indicates errors/failures
	// Color: Red, Icon: "x"
	StatusError = lipgloss.AdaptiveColor{Light: "#EF4444", Dark: "#EF4444"}

	// StatusPaused indicates inactive/paused state
	// Color: Gray, Icon: "||"
	StatusPaused = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#6B7280"}
)

// UI chrome colors - structural elements
var (
	// Primary is the accent/focus color
	Primary = lipgloss.AdaptiveColor{Light: "#7D56F4", Dark: "#7D56F4"}

	// Border is the default border color
	Border = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#3C3C3C"}

	// BorderFocus is the border color for focused elements
	BorderFocus = lipgloss.AdaptiveColor{Light: "#7D56F4", Dark: "#7D56F4"}

	// TextPrimary is the main text color
	TextPrimary = lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"}

	// TextSecondary is for secondary text (descriptions, labels)
	TextSecondary = lipgloss.AdaptiveColor{Light: "#4B5563", Dark: "#9CA3AF"}

	// TextMuted is for hints and subtle text
	TextMuted = lipgloss.AdaptiveColor{Light: "#9CA3AF", Dark: "#6B7280"}

	// Background is the main background color
	Background = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1a1a1a"}

	// BackgroundSubtle is for cards, overlays, etc.
	BackgroundSubtle = lipgloss.AdaptiveColor{Light: "#F3F4F6", Dark: "#2a2a2a"}

	// BackgroundSelected is for selected items
	BackgroundSelected = lipgloss.AdaptiveColor{Light: "#dde4f0", Dark: "#3C3C4C"}
)

// Status icons for accessibility (shape + color)
const (
	IconSuccess = "+"
	IconRunning = "○" // Use with spinner in actual UI
	IconWarning = "!"
	IconError   = "×"
	IconPaused  = "⏸"
	IconReady   = "●"
)

// Pre-built styles for common UI elements

// StatusStyles contains pre-built styles for each status type
var StatusStyles = struct {
	Success lipgloss.Style
	Running lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style
	Paused  lipgloss.Style
}{
	Success: lipgloss.NewStyle().Foreground(StatusSuccess),
	Running: lipgloss.NewStyle().Foreground(StatusRunning),
	Warning: lipgloss.NewStyle().Foreground(StatusWarning),
	Error:   lipgloss.NewStyle().Foreground(StatusError),
	Paused:  lipgloss.NewStyle().Foreground(StatusPaused),
}

// TextStyles contains pre-built styles for text elements
var TextStyles = struct {
	Primary   lipgloss.Style
	Secondary lipgloss.Style
	Muted     lipgloss.Style
}{
	Primary:   lipgloss.NewStyle().Foreground(TextPrimary),
	Secondary: lipgloss.NewStyle().Foreground(TextSecondary),
	Muted:     lipgloss.NewStyle().Foreground(TextMuted),
}

// BorderStyles contains pre-built styles for bordered elements
var BorderStyles = struct {
	Default lipgloss.Style
	Focus   lipgloss.Style
	Rounded lipgloss.Style
}{
	Default: lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(Border),
	Focus: lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(BorderFocus),
	Rounded: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Border),
}

// BadgeStyle creates a styled badge with the given color
func BadgeStyle(color lipgloss.TerminalColor) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(color).
		Padding(0, 1)
}

// StatusBadge returns a formatted status badge string
func StatusBadge(status string, color lipgloss.TerminalColor) string {
	return BadgeStyle(color).Render(status)
}

// Spacing constants for consistent layout
const (
	SpaceXS = 1
	SpaceSM = 2
	SpaceMD = 4
	SpaceLG = 8
	SpaceXL = 16
)

// OverlayStyle creates a style for overlay/modal containers
func OverlayStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BorderFocus).
		Padding(1, 2).
		Background(BackgroundSubtle)
}

// CardStyle creates a style for card-like containers
func CardStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Border).
		Padding(1, 2)
}

// FocusedCardStyle creates a style for focused card-like containers
func FocusedCardStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BorderFocus).
		Padding(1, 2)
}
