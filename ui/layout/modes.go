// Package layout provides responsive layout calculations for the TUI.
package layout

// LayoutMode represents the current layout mode based on terminal dimensions.
type LayoutMode int

const (
	// LayoutFull is for large terminals (>= 120w x 40h).
	// Shows all components with generous spacing.
	LayoutFull LayoutMode = iota

	// LayoutStandard is for medium terminals (>= 100w x 30h).
	// Default comfortable layout.
	LayoutStandard

	// LayoutCompact is for smaller terminals (>= 80w x 24h).
	// Reduced spacing, compact list items.
	LayoutCompact

	// LayoutMinimal is for terminals below minimum size.
	// Shows warning or uses vertical stacking.
	LayoutMinimal
)

// String returns the string representation of the layout mode.
func (m LayoutMode) String() string {
	switch m {
	case LayoutFull:
		return "full"
	case LayoutStandard:
		return "standard"
	case LayoutCompact:
		return "compact"
	case LayoutMinimal:
		return "minimal"
	default:
		return "unknown"
	}
}

// DetermineMode calculates the appropriate layout mode for the given dimensions.
func DetermineMode(width, height int) LayoutMode {
	// Check for minimal first (below absolute minimum)
	if width < MinWidth || height < MinHeight {
		return LayoutMinimal
	}

	// Determine mode based on both width and height
	// Use the more restrictive dimension
	widthMode := determineWidthMode(width)
	heightMode := determineHeightMode(height)

	// Return the more restrictive (higher value = more restrictive)
	if widthMode > heightMode {
		return widthMode
	}
	return heightMode
}

func determineWidthMode(width int) LayoutMode {
	switch {
	case width >= FullWidth:
		return LayoutFull
	case width >= StandardWidth:
		return LayoutStandard
	case width >= MinWidth:
		return LayoutCompact
	default:
		return LayoutMinimal
	}
}

func determineHeightMode(height int) LayoutMode {
	switch {
	case height >= FullHeight:
		return LayoutFull
	case height >= StandardHeight:
		return LayoutStandard
	case height >= MinHeight:
		return LayoutCompact
	default:
		return LayoutMinimal
	}
}
