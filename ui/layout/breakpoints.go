package layout

// Width breakpoints
const (
	// MinWidth is the absolute minimum terminal width.
	MinWidth = 80

	// CompactWidth triggers compact mode features.
	CompactWidth = 100

	// StandardWidth is the threshold for standard layout.
	StandardWidth = 120

	// FullWidth is the threshold for full layout with all features.
	FullWidth = 140
)

// Height breakpoints
const (
	// MinHeight is the absolute minimum terminal height (standard terminal).
	MinHeight = 24

	// CompactHeight triggers compact mode features.
	CompactHeight = 30

	// StandardHeight is the threshold for standard layout.
	StandardHeight = 40

	// FullHeight is the threshold for full layout.
	FullHeight = 50
)

// List panel constraints
const (
	// ListMinWidth is the minimum width for the list panel.
	ListMinWidth = 25

	// ListMaxWidth is the maximum width for the list panel (prevents over-stretching).
	ListMaxWidth = 50

	// ListCompactWidth is the list width in compact mode.
	ListCompactWidth = 30
)

// Menu constraints
const (
	// MenuMinHeight is the minimum menu height (1 line + border).
	MenuMinHeight = 2

	// MenuStandardHeight is the standard menu height.
	MenuStandardHeight = 3

	// MenuMaxHeight is the maximum menu height.
	MenuMaxHeight = 5
)

// Component constraints
const (
	// ErrBoxHeight is the fixed error box height.
	ErrBoxHeight = 1

	// TitleAreaHeight is the list title area height in normal mode.
	TitleAreaHeight = 5

	// TitleAreaHeightCompact is the list title area height in compact mode.
	TitleAreaHeightCompact = 3
)

// Overlay constraints
const (
	// OverlayMaxWidth is the maximum overlay width.
	OverlayMaxWidth = 80

	// OverlayMaxHeight is the maximum overlay height.
	OverlayMaxHeight = 25

	// OverlayMinWidth is the minimum overlay width.
	OverlayMinWidth = 40

	// OverlayMinHeight is the minimum overlay height.
	OverlayMinHeight = 10

	// OverlayMargin is the minimum margin from terminal edges.
	OverlayMargin = 4
)
