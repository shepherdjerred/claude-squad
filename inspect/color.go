package inspect

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// ExtractStyleInfo extracts style information from a lipgloss style.
func ExtractStyleInfo(style lipgloss.Style, styleNames ...string) *StyleInfo {
	info := &StyleInfo{
		Foreground:    colorToString(style.GetForeground()),
		Background:    colorToString(style.GetBackground()),
		Bold:          style.GetBold(),
		Italic:        style.GetItalic(),
		Underline:     style.GetUnderline(),
		AppliedStyles: styleNames,
	}

	// Extract padding [top, right, bottom, left]
	top := style.GetPaddingTop()
	right := style.GetPaddingRight()
	bottom := style.GetPaddingBottom()
	left := style.GetPaddingLeft()
	if top > 0 || right > 0 || bottom > 0 || left > 0 {
		info.Padding = []int{top, right, bottom, left}
	}

	// Extract border if present
	if style.GetBorderTop() || style.GetBorderRight() || style.GetBorderBottom() || style.GetBorderLeft() {
		info.Border = "rounded" // Assuming rounded, could detect actual style
		info.BorderColor = colorToString(style.GetBorderTopForeground())
	}

	return info
}

// colorToString converts a lipgloss.TerminalColor to a string representation.
func colorToString(c lipgloss.TerminalColor) string {
	if c == nil {
		return ""
	}

	// Handle different color types
	switch v := c.(type) {
	case lipgloss.Color:
		return string(v)
	case lipgloss.AdaptiveColor:
		return fmt.Sprintf("adaptive(light=%s, dark=%s)", v.Light, v.Dark)
	case lipgloss.CompleteColor:
		return fmt.Sprintf("complete(true=%s, ansi=%s, ansi256=%s)",
			v.TrueColor, v.ANSI, v.ANSI256)
	case lipgloss.CompleteAdaptiveColor:
		return "complete_adaptive"
	default:
		return fmt.Sprintf("%v", c)
	}
}

// StyleRegistry tracks named styles for inspection.
type StyleRegistry struct {
	styles map[string]lipgloss.Style
}

var globalRegistry = &StyleRegistry{
	styles: make(map[string]lipgloss.Style),
}

// RegisterStyle registers a named style for inspection.
func RegisterStyle(name string, style lipgloss.Style) {
	globalRegistry.styles[name] = style
}

// GetRegisteredStyle retrieves a registered style by name.
func GetRegisteredStyle(name string) (lipgloss.Style, bool) {
	style, ok := globalRegistry.styles[name]
	return style, ok
}

// GetAllStyles returns info for all registered styles.
func GetAllStyles() map[string]*StyleInfo {
	result := make(map[string]*StyleInfo)
	for name, style := range globalRegistry.styles {
		result[name] = ExtractStyleInfo(style, name)
	}
	return result
}

// ListRegisteredStyles returns the names of all registered styles.
func ListRegisteredStyles() []string {
	names := make([]string, 0, len(globalRegistry.styles))
	for name := range globalRegistry.styles {
		names = append(names, name)
	}
	return names
}
