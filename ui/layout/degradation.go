package layout

// Degradation holds flags indicating which UI features should be hidden or simplified.
// Features are listed in order of degradation priority (first to hide).
type Degradation struct {
	// List item degradation
	HideListSummaries    bool // Hide summary lines in list items (height < 35)
	HideListDescriptions bool // Hide branch info line (height < 30)
	HideTimerInfo        bool // Hide age/timer in list (width < 100)

	// Component simplification
	SimplifyTabs         bool // Remove fancy tab borders (width < 90)
	SingleLineMenu       bool // Compact menu to one line (height < 26)
	HideScrollIndicators bool // Remove scroll hints (height < 28)

	// Critical degradation
	HideLogoArt      bool // Hide ASCII art in fallback (height < 20 or width < 50)
	ShowMinWarning   bool // Terminal too small warning (below MinWidth/MinHeight)
	UseVerticalStack bool // Stack panels vertically (width < 80)
}

// Threshold constants for degradation
const (
	SummaryHideHeight      = 35
	DescriptionHideHeight  = 30
	TimerHideWidth         = 100
	TabSimplifyWidth       = 90
	SingleLineMenuHeight   = 26
	ScrollIndicatorHeight  = 28
	LogoHideHeight         = 20
	LogoHideWidth          = 50
	VerticalStackWidth     = 80
)

// ComputeDegradation calculates which UI features should be degraded.
func ComputeDegradation(c Constraints) Degradation {
	return Degradation{
		// List item degradation
		HideListSummaries:    c.TerminalHeight < SummaryHideHeight,
		HideListDescriptions: c.TerminalHeight < DescriptionHideHeight,
		HideTimerInfo:        c.TerminalWidth < TimerHideWidth,

		// Component simplification
		SimplifyTabs:         c.TerminalWidth < TabSimplifyWidth,
		SingleLineMenu:       c.TerminalHeight < SingleLineMenuHeight,
		HideScrollIndicators: c.TerminalHeight < ScrollIndicatorHeight,

		// Critical degradation
		HideLogoArt:      c.TerminalHeight < LogoHideHeight || c.TerminalWidth < LogoHideWidth,
		ShowMinWarning:   c.ShowMinWarning,
		UseVerticalStack: c.UseVerticalStack,
	}
}

// IsCompactMode returns true if the layout should use compact rendering.
func (d Degradation) IsCompactMode() bool {
	return d.HideListSummaries || d.HideListDescriptions
}

// ShouldShowSummary returns true if list item summaries should be shown.
func (d Degradation) ShouldShowSummary() bool {
	return !d.HideListSummaries
}

// ShouldShowDescription returns true if list item descriptions should be shown.
func (d Degradation) ShouldShowDescription() bool {
	return !d.HideListDescriptions
}

// ShouldShowTimer returns true if timer/age info should be shown.
func (d Degradation) ShouldShowTimer() bool {
	return !d.HideTimerInfo
}

// GetTitleAreaHeight returns the appropriate title area height.
func (d Degradation) GetTitleAreaHeight() int {
	if d.IsCompactMode() {
		return TitleAreaHeightCompact
	}
	return TitleAreaHeight
}
