package inspect

import (
	"fmt"
	"strings"
	"time"

	"claude-squad/ui/layout"
)

// Snapshot represents a complete UI state at a point in time.
type Snapshot struct {
	// Timestamp when the snapshot was taken.
	Timestamp time.Time `json:"timestamp"`

	// Version of the snapshot format.
	Version string `json:"version"`

	// Terminal contains terminal dimensions.
	Terminal TerminalInfo `json:"terminal"`

	// AppState contains application state information.
	AppState AppStateInfo `json:"app_state"`

	// Layout contains layout configuration.
	Layout LayoutInfo `json:"layout"`

	// Components is the root of the component tree.
	Components *Node `json:"components"`

	// Breakpoints contains information about responsive breakpoints.
	Breakpoints []BreakpointInfo `json:"breakpoints"`
}

// TerminalInfo contains terminal dimensions.
type TerminalInfo struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// AppStateInfo contains application-level state.
type AppStateInfo struct {
	// State is the current app state (e.g., "default", "new", "prompt").
	State string `json:"state"`

	// HasOverlay indicates if an overlay is currently displayed.
	HasOverlay bool `json:"has_overlay"`

	// OverlayType is the type of overlay if one is displayed.
	OverlayType string `json:"overlay_type,omitempty"`

	// InstanceCount is the total number of instances.
	InstanceCount int `json:"instance_count"`

	// SelectedIndex is the currently selected instance index.
	SelectedIndex int `json:"selected_index"`

	// ErrorMessage is the current error message if any.
	ErrorMessage string `json:"error_message,omitempty"`
}

// LayoutInfo contains layout configuration.
type LayoutInfo struct {
	// Mode is the current layout mode.
	Mode string `json:"mode"`

	// ListWidth is the list panel width.
	ListWidth int `json:"list_width"`

	// ListHeight is the list panel height.
	ListHeight int `json:"list_height"`

	// PreviewWidth is the preview panel width.
	PreviewWidth int `json:"preview_width"`

	// PreviewHeight is the preview panel height.
	PreviewHeight int `json:"preview_height"`

	// MenuHeight is the menu height.
	MenuHeight int `json:"menu_height"`

	// UseVerticalStack indicates if panels are stacked vertically.
	UseVerticalStack bool `json:"use_vertical_stack"`

	// Degradation contains active degradation flags.
	Degradation DegradationInfo `json:"degradation"`
}

// DegradationInfo contains active UI degradation flags.
type DegradationInfo struct {
	HideListSummaries    bool `json:"hide_list_summaries"`
	HideListDescriptions bool `json:"hide_list_descriptions"`
	HideTimerInfo        bool `json:"hide_timer_info"`
	SimplifyTabs         bool `json:"simplify_tabs"`
	SingleLineMenu       bool `json:"single_line_menu"`
	HideScrollIndicators bool `json:"hide_scroll_indicators"`
	HideLogoArt          bool `json:"hide_logo_art"`
	ShowMinWarning       bool `json:"show_min_warning"`
}

// BreakpointInfo contains information about a responsive breakpoint.
type BreakpointInfo struct {
	// Name is the breakpoint name.
	Name string `json:"name"`

	// Threshold is the dimension threshold.
	Threshold int `json:"threshold"`

	// Active indicates if this breakpoint is currently triggered.
	Active bool `json:"active"`

	// Dimension is "width" or "height".
	Dimension string `json:"dimension"`
}

// NewSnapshot creates a new snapshot with current timestamp.
func NewSnapshot() *Snapshot {
	return &Snapshot{
		Timestamp: time.Now(),
		Version:   "1.0.0",
	}
}

// WithTerminal sets terminal info and returns the snapshot for chaining.
func (s *Snapshot) WithTerminal(width, height int) *Snapshot {
	s.Terminal = TerminalInfo{Width: width, Height: height}
	return s
}

// WithLayout sets layout info from constraints and degradation.
func (s *Snapshot) WithLayout(c layout.Constraints, d layout.Degradation) *Snapshot {
	s.Layout = LayoutInfo{
		Mode:             c.Mode.String(),
		ListWidth:        c.ListWidth,
		ListHeight:       c.ListHeight,
		PreviewWidth:     c.PreviewWidth,
		PreviewHeight:    c.PreviewHeight,
		MenuHeight:       c.MenuHeight,
		UseVerticalStack: c.UseVerticalStack,
		Degradation: DegradationInfo{
			HideListSummaries:    d.HideListSummaries,
			HideListDescriptions: d.HideListDescriptions,
			HideTimerInfo:        d.HideTimerInfo,
			SimplifyTabs:         d.SimplifyTabs,
			SingleLineMenu:       d.SingleLineMenu,
			HideScrollIndicators: d.HideScrollIndicators,
			HideLogoArt:          d.HideLogoArt,
			ShowMinWarning:       d.ShowMinWarning,
		},
	}

	// Add breakpoint information
	s.Breakpoints = []BreakpointInfo{
		{Name: "hide_summaries", Threshold: layout.SummaryHideHeight, Active: d.HideListSummaries, Dimension: "height"},
		{Name: "hide_descriptions", Threshold: layout.DescriptionHideHeight, Active: d.HideListDescriptions, Dimension: "height"},
		{Name: "hide_timer", Threshold: layout.TimerHideWidth, Active: d.HideTimerInfo, Dimension: "width"},
		{Name: "simplify_tabs", Threshold: layout.TabSimplifyWidth, Active: d.SimplifyTabs, Dimension: "width"},
		{Name: "single_line_menu", Threshold: layout.SingleLineMenuHeight, Active: d.SingleLineMenu, Dimension: "height"},
		{Name: "vertical_stack", Threshold: layout.VerticalStackWidth, Active: d.UseVerticalStack, Dimension: "width"},
	}

	return s
}

// WithComponents sets the component tree root.
func (s *Snapshot) WithComponents(root *Node) *Snapshot {
	s.Components = root
	return s
}

// ToText returns a human-readable text representation.
func (s *Snapshot) ToText() string {
	var b strings.Builder

	b.WriteString("=== UI Snapshot ===\n")
	b.WriteString(fmt.Sprintf("Time: %s\n", s.Timestamp.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("Terminal: %dx%d\n", s.Terminal.Width, s.Terminal.Height))
	b.WriteString(fmt.Sprintf("State: %s\n", s.AppState.State))

	b.WriteString("\n--- Layout ---\n")
	b.WriteString(fmt.Sprintf("Mode: %s\n", s.Layout.Mode))
	b.WriteString(fmt.Sprintf("List: %dx%d\n", s.Layout.ListWidth, s.Layout.ListHeight))
	b.WriteString(fmt.Sprintf("Preview: %dx%d\n", s.Layout.PreviewWidth, s.Layout.PreviewHeight))
	b.WriteString(fmt.Sprintf("Vertical Stack: %v\n", s.Layout.UseVerticalStack))

	b.WriteString("\n--- Active Breakpoints ---\n")
	for _, bp := range s.Breakpoints {
		status := "[ ]"
		if bp.Active {
			status = "[X]"
		}
		b.WriteString(fmt.Sprintf("  %s %s (threshold: %d %s)\n", status, bp.Name, bp.Threshold, bp.Dimension))
	}

	if s.Components != nil {
		b.WriteString("\n--- Components ---\n")
		writeNodeText(&b, s.Components, 0)
	}

	return b.String()
}

func writeNodeText(b *strings.Builder, node *Node, indent int) {
	prefix := strings.Repeat("  ", indent)

	b.WriteString(fmt.Sprintf("%s%s", prefix, node.Type))
	if node.ID != "" {
		b.WriteString(fmt.Sprintf(" [%s]", node.ID))
	}
	b.WriteString(fmt.Sprintf(" (%dx%d)", node.Bounds.Width, node.Bounds.Height))

	if node.Truncated != nil {
		b.WriteString(fmt.Sprintf(" TRUNCATED(%d->%d)",
			node.Truncated.OriginalLength,
			node.Truncated.DisplayLength))
	}

	b.WriteString("\n")

	for _, child := range node.Children {
		writeNodeText(b, child, indent+1)
	}
}
