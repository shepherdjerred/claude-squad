package layout

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMinimumTerminal80x24 verifies all behavior at the minimum supported terminal size
func TestMinimumTerminal80x24(t *testing.T) {
	width := 80
	height := 24

	t.Run("mode is compact", func(t *testing.T) {
		mode := DetermineMode(width, height)
		assert.Equal(t, LayoutCompact, mode, "80x24 should be compact mode")
	})

	t.Run("constraints are valid", func(t *testing.T) {
		c := ComputeConstraints(width, height)

		// Verify mode
		assert.Equal(t, LayoutCompact, c.Mode)

		// Verify no minimal warnings
		assert.False(t, c.ShowMinWarning, "80x24 should not show warning")
		assert.False(t, c.UseVerticalStack, "80x24 should not use vertical stack")

		// Verify all dimensions are positive
		assert.Positive(t, c.ListWidth, "ListWidth")
		assert.Positive(t, c.ListHeight, "ListHeight")
		assert.Positive(t, c.PreviewWidth, "PreviewWidth")
		assert.Positive(t, c.PreviewHeight, "PreviewHeight")
		assert.Positive(t, c.MenuWidth, "MenuWidth")
		assert.Positive(t, c.MenuHeight, "MenuHeight")
		assert.Positive(t, c.ErrBoxWidth, "ErrBoxWidth")
		assert.Positive(t, c.ErrBoxHeight, "ErrBoxHeight")

		// Verify widths don't exceed terminal
		assert.LessOrEqual(t, c.ListWidth+c.PreviewWidth, width, "total width should not exceed terminal")

		// Verify heights fit
		totalHeight := c.ListHeight + c.MenuHeight + c.ErrBoxHeight
		assert.LessOrEqual(t, totalHeight, height, "total height should not exceed terminal")
	})

	t.Run("degradation flags are set correctly", func(t *testing.T) {
		c := ComputeConstraints(width, height)
		d := ComputeDegradation(c)

		// At 80x24:
		// - height 24 < 35: HideListSummaries = true
		// - height 24 < 30: HideListDescriptions = true
		// - width 80 < 100: HideTimerInfo = true
		// - width 80 < 90: SimplifyTabs = true
		// - height 24 < 26: SingleLineMenu = true
		// - height 24 < 28: HideScrollIndicators = true

		assert.True(t, d.HideListSummaries, "should hide summaries at 80x24")
		assert.True(t, d.HideListDescriptions, "should hide descriptions at 80x24")
		assert.True(t, d.HideTimerInfo, "should hide timer at 80x24")
		assert.True(t, d.SimplifyTabs, "should simplify tabs at 80x24")
		assert.True(t, d.SingleLineMenu, "should use single line menu at 80x24")
		assert.True(t, d.HideScrollIndicators, "should hide scroll indicators at 80x24")

		// Critical degradation should NOT be triggered
		assert.False(t, d.ShowMinWarning, "should not show min warning at 80x24")
		assert.False(t, d.UseVerticalStack, "should not use vertical stack at 80x24")
	})

	t.Run("helper methods work correctly", func(t *testing.T) {
		c := ComputeConstraints(width, height)
		d := ComputeDegradation(c)

		assert.True(t, d.IsCompactMode(), "should be in compact mode")
		assert.False(t, d.ShouldShowSummary(), "should not show summary")
		assert.False(t, d.ShouldShowDescription(), "should not show description")
		assert.False(t, d.ShouldShowTimer(), "should not show timer")
	})
}

// TestBelowMinimumTerminal verifies behavior at sizes smaller than 80x24
func TestBelowMinimumTerminal(t *testing.T) {
	tests := []struct {
		name              string
		width             int
		height            int
		expectMinWarning  bool
		expectVertStack   bool
		expectMinimalMode bool
	}{
		{
			name:              "narrow terminal 70x30",
			width:             70,
			height:            30,
			expectMinWarning:  true,
			expectVertStack:   true, // width < 80
			expectMinimalMode: true,
		},
		{
			name:              "short terminal 100x20",
			width:             100,
			height:            20,
			expectMinWarning:  true,
			expectVertStack:   false, // width >= 80
			expectMinimalMode: true,
		},
		{
			name:              "tiny terminal 60x15",
			width:             60,
			height:            15,
			expectMinWarning:  true,
			expectVertStack:   true,
			expectMinimalMode: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ComputeConstraints(tt.width, tt.height)
			d := ComputeDegradation(c)

			assert.Equal(t, tt.expectMinWarning, c.ShowMinWarning, "ShowMinWarning")
			assert.Equal(t, tt.expectVertStack, c.UseVerticalStack, "UseVerticalStack")

			if tt.expectMinimalMode {
				assert.Equal(t, LayoutMinimal, c.Mode, "Mode should be minimal")
			}

			// All dimensions should still be positive even in degraded mode
			assert.Positive(t, c.ListWidth, "ListWidth should be positive")
			assert.Positive(t, c.ListHeight, "ListHeight should be positive")
			assert.Positive(t, c.PreviewWidth, "PreviewWidth should be positive")
			assert.Positive(t, c.PreviewHeight, "PreviewHeight should be positive")

			// Degradation based on size thresholds
			assert.True(t, d.HideListSummaries, "should hide summaries at these sizes")
			if tt.height < 30 {
				assert.True(t, d.HideListDescriptions, "should hide descriptions when height < 30")
			}
		})
	}
}

// TestDegradationProgression verifies gradual degradation as terminal shrinks
func TestDegradationProgression(t *testing.T) {
	// Full terminal - no degradation
	t.Run("full terminal 150x50", func(t *testing.T) {
		c := ComputeConstraints(150, 50)
		d := ComputeDegradation(c)

		assert.False(t, d.HideListSummaries)
		assert.False(t, d.HideListDescriptions)
		assert.False(t, d.HideTimerInfo)
		assert.False(t, d.SimplifyTabs)
		assert.False(t, d.SingleLineMenu)
		assert.False(t, d.HideScrollIndicators)
	})

	// Standard terminal - no degradation
	t.Run("standard terminal 120x40", func(t *testing.T) {
		c := ComputeConstraints(120, 40)
		d := ComputeDegradation(c)

		assert.False(t, d.HideListSummaries)
		assert.False(t, d.HideListDescriptions)
		assert.False(t, d.HideTimerInfo)
		assert.False(t, d.SimplifyTabs)
		assert.False(t, d.SingleLineMenu)
		assert.False(t, d.HideScrollIndicators)
	})

	// Moderately small - some degradation
	t.Run("compact terminal 90x32", func(t *testing.T) {
		c := ComputeConstraints(90, 32)
		d := ComputeDegradation(c)

		assert.True(t, d.HideListSummaries, "should hide summaries (height < 35)")
		assert.False(t, d.HideListDescriptions, "should show descriptions (height >= 30)")
		assert.True(t, d.HideTimerInfo, "should hide timer (width < 100)")
		assert.False(t, d.SimplifyTabs, "should not simplify tabs (width >= 90)")
	})

	// Small terminal - more degradation
	t.Run("small terminal 85x27", func(t *testing.T) {
		c := ComputeConstraints(85, 27)
		d := ComputeDegradation(c)

		assert.True(t, d.HideListSummaries, "should hide summaries (height < 35)")
		assert.True(t, d.HideListDescriptions, "should hide descriptions (height < 30)")
		assert.True(t, d.HideTimerInfo, "should hide timer (width < 100)")
		assert.True(t, d.SimplifyTabs, "should simplify tabs (width < 90)")
		assert.False(t, d.SingleLineMenu, "should not use single line (height >= 26)")
		assert.True(t, d.HideScrollIndicators, "should hide scroll (height < 28)")
	})
}
