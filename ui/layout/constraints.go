package layout

// Constraints holds the computed layout constraints for all components.
type Constraints struct {
	// Terminal dimensions
	TerminalWidth  int
	TerminalHeight int

	// Computed mode
	Mode LayoutMode

	// Panel dimensions (computed)
	ListWidth      int
	ListHeight     int
	PreviewWidth   int
	PreviewHeight  int
	MenuWidth      int
	MenuHeight     int
	ErrBoxWidth    int
	ErrBoxHeight   int

	// Layout flags
	UseVerticalStack bool // Stack list on top of preview for very narrow terminals
	ShowMinWarning   bool // Terminal is below minimum size
}

// ComputeConstraints calculates layout constraints for the given terminal dimensions.
func ComputeConstraints(width, height int) Constraints {
	c := Constraints{
		TerminalWidth:  width,
		TerminalHeight: height,
	}

	// 1. Determine layout mode
	c.Mode = DetermineMode(width, height)

	// 2. Check for minimum size violation
	if width < MinWidth || height < MinHeight {
		c.ShowMinWarning = true
		// Still compute basic layout for partial display
	}

	// 3. Compute menu and error box heights (fixed elements)
	c.ErrBoxHeight = ErrBoxHeight
	c.ErrBoxWidth = width
	c.MenuHeight = computeMenuHeight(c.Mode, height)
	c.MenuWidth = width

	// 4. Compute content area height
	contentHeight := height - c.MenuHeight - c.ErrBoxHeight

	// 5. Compute horizontal distribution
	if c.Mode == LayoutMinimal && width < MinWidth {
		// Vertical stacking for very narrow terminals
		c.UseVerticalStack = true
		c.ListWidth = width
		c.ListHeight = contentHeight / 3
		c.PreviewWidth = width
		c.PreviewHeight = contentHeight - c.ListHeight
	} else {
		// Horizontal layout with constrained list width
		c.ListWidth = computeListWidth(width, c.Mode)
		c.PreviewWidth = width - c.ListWidth
		c.ListHeight = contentHeight
		c.PreviewHeight = contentHeight
	}

	return c
}

// computeListWidth calculates the list panel width based on mode.
func computeListWidth(totalWidth int, mode LayoutMode) int {
	var targetPercent float32
	var minWidth, maxWidth int

	switch mode {
	case LayoutFull:
		targetPercent = 0.25 // 25% for full mode
		minWidth = ListMinWidth
		maxWidth = ListMaxWidth
	case LayoutStandard:
		targetPercent = 0.30 // 30% for standard mode
		minWidth = ListMinWidth
		maxWidth = ListMaxWidth
	case LayoutCompact:
		targetPercent = 0.35 // 35% for compact - need more relative space
		minWidth = ListMinWidth
		maxWidth = ListCompactWidth
	default:
		// Minimal mode
		return ListMinWidth
	}

	computed := int(float32(totalWidth) * targetPercent)
	return clamp(computed, minWidth, maxWidth)
}

// computeMenuHeight calculates the menu height based on mode.
func computeMenuHeight(mode LayoutMode, totalHeight int) int {
	switch mode {
	case LayoutFull:
		return MenuMaxHeight
	case LayoutStandard:
		return MenuStandardHeight
	case LayoutCompact:
		return MenuMinHeight
	default:
		return MenuMinHeight
	}
}

// ComputeOverlaySize calculates constrained overlay dimensions.
func ComputeOverlaySize(termWidth, termHeight int, preferredWidth, preferredHeight int) (int, int) {
	maxW := termWidth - OverlayMargin*2
	maxH := termHeight - OverlayMargin*2

	w := clamp(preferredWidth, OverlayMinWidth, min(maxW, OverlayMaxWidth))
	h := clamp(preferredHeight, OverlayMinHeight, min(maxH, OverlayMaxHeight))

	return w, h
}

// Helper functions

func clamp(value, minVal, maxVal int) int {
	if value < minVal {
		return minVal
	}
	if value > maxVal {
		return maxVal
	}
	return value
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
