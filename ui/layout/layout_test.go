package layout

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetermineMode(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
		want   LayoutMode
	}{
		{
			name:   "full mode - large terminal",
			width:  140,
			height: 50,
			want:   LayoutFull,
		},
		{
			name:   "standard mode - both at standard thresholds",
			width:  120,
			height: 40,
			want:   LayoutStandard,
		},
		{
			name:   "compact mode - medium terminal",
			width:  110,
			height: 35,
			want:   LayoutCompact, // width >= 100 (Compact) but < 120 (Standard), height >= 30 but < 40
		},
		{
			name:   "compact mode - small terminal",
			width:  85,
			height: 26,
			want:   LayoutCompact,
		},
		{
			name:   "minimal mode - below minimum width",
			width:  70,
			height: 30,
			want:   LayoutMinimal,
		},
		{
			name:   "minimal mode - below minimum height",
			width:  100,
			height: 20,
			want:   LayoutMinimal,
		},
		{
			name:   "minimal mode - both below minimum",
			width:  70,
			height: 20,
			want:   LayoutMinimal,
		},
		{
			name:   "exact minimum - should be compact",
			width:  80,
			height: 24,
			want:   LayoutCompact,
		},
		{
			name:   "wide but short",
			width:  150,
			height: 25,
			want:   LayoutCompact, // Uses most restrictive mode (height-based)
		},
		{
			name:   "tall but narrow",
			width:  85,
			height: 60,
			want:   LayoutCompact, // Uses most restrictive mode (width-based)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineMode(tt.width, tt.height)
			assert.Equal(t, tt.want, got, "DetermineMode(%d, %d)", tt.width, tt.height)
		})
	}
}

func TestComputeConstraints(t *testing.T) {
	tests := []struct {
		name                 string
		width                int
		height               int
		wantMode             LayoutMode
		wantVerticalStack    bool
		wantShowMinWarning   bool
		wantListWidthGreater int
	}{
		{
			name:                 "standard terminal",
			width:                120,
			height:               40,
			wantMode:             LayoutStandard, // 120 >= StandardWidth, 40 >= StandardHeight
			wantVerticalStack:    false,
			wantShowMinWarning:   false,
			wantListWidthGreater: 30,
		},
		{
			name:                 "minimum terminal",
			width:                80,
			height:               24,
			wantMode:             LayoutCompact,
			wantVerticalStack:    false,
			wantShowMinWarning:   false,
			wantListWidthGreater: 20,
		},
		{
			name:               "below minimum width - vertical stack",
			width:              70,
			height:             30,
			wantMode:           LayoutMinimal,
			wantVerticalStack:  true,
			wantShowMinWarning: true,
		},
		{
			name:               "below minimum height",
			width:              100,
			height:             20,
			wantMode:           LayoutMinimal,
			wantVerticalStack:  false,
			wantShowMinWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ComputeConstraints(tt.width, tt.height)
			assert.Equal(t, tt.wantMode, c.Mode, "Mode")
			assert.Equal(t, tt.wantVerticalStack, c.UseVerticalStack, "UseVerticalStack")
			assert.Equal(t, tt.wantShowMinWarning, c.ShowMinWarning, "ShowMinWarning")

			if tt.wantListWidthGreater > 0 {
				assert.Greater(t, c.ListWidth, tt.wantListWidthGreater, "ListWidth should be greater than %d", tt.wantListWidthGreater)
			}

			// Verify dimensions are positive
			assert.Positive(t, c.ListWidth, "ListWidth should be positive")
			assert.Positive(t, c.ListHeight, "ListHeight should be positive")
			assert.Positive(t, c.PreviewWidth, "PreviewWidth should be positive")
			assert.Positive(t, c.PreviewHeight, "PreviewHeight should be positive")
			assert.Positive(t, c.MenuHeight, "MenuHeight should be positive")
		})
	}
}

func TestComputeDegradation(t *testing.T) {
	tests := []struct {
		name                     string
		width                    int
		height                   int
		wantHideSummaries        bool
		wantHideDescriptions     bool
		wantHideTimer            bool
		wantSingleLineMenu       bool
		wantSimplifyTabs         bool
		wantHideScrollIndicators bool
	}{
		{
			name:                     "large terminal - no degradation",
			width:                    150,
			height:                   50,
			wantHideSummaries:        false,
			wantHideDescriptions:     false,
			wantHideTimer:            false,
			wantSingleLineMenu:       false,
			wantSimplifyTabs:         false,
			wantHideScrollIndicators: false,
		},
		{
			name:              "narrow terminal - hide timer, simplify tabs",
			width:             85,
			height:            40,
			wantHideSummaries: false,
			wantHideTimer:     true,
			wantSimplifyTabs:  true,
		},
		{
			name:                 "short terminal - hide summaries",
			width:                120,
			height:               32,
			wantHideSummaries:    true,
			wantHideDescriptions: false,
			wantSingleLineMenu:   false,
		},
		{
			name:                     "very short terminal - hide descriptions, single line menu",
			width:                    120,
			height:                   25,
			wantHideSummaries:        true,
			wantHideDescriptions:     true,
			wantSingleLineMenu:       true,
			wantHideScrollIndicators: true, // height 25 < 28 (ScrollIndicatorHeight)
		},
		{
			name:                     "compact terminal - multiple degradations",
			width:                    85,
			height:                   25,
			wantHideSummaries:        true,
			wantHideDescriptions:     true,
			wantHideTimer:            true,
			wantSingleLineMenu:       true,
			wantSimplifyTabs:         true,
			wantHideScrollIndicators: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ComputeConstraints(tt.width, tt.height)
			d := ComputeDegradation(c)

			assert.Equal(t, tt.wantHideSummaries, d.HideListSummaries, "HideListSummaries")
			assert.Equal(t, tt.wantHideDescriptions, d.HideListDescriptions, "HideListDescriptions")
			assert.Equal(t, tt.wantHideTimer, d.HideTimerInfo, "HideTimerInfo")
			assert.Equal(t, tt.wantSingleLineMenu, d.SingleLineMenu, "SingleLineMenu")
			assert.Equal(t, tt.wantSimplifyTabs, d.SimplifyTabs, "SimplifyTabs")
			assert.Equal(t, tt.wantHideScrollIndicators, d.HideScrollIndicators, "HideScrollIndicators")
		})
	}
}

func TestIsCompactMode(t *testing.T) {
	tests := []struct {
		name           string
		degradation    Degradation
		wantCompact    bool
		wantShowDesc   bool
		wantShowTimer  bool
	}{
		{
			name:           "no degradation",
			degradation:    Degradation{},
			wantCompact:    false,
			wantShowDesc:   true,
			wantShowTimer:  true,
		},
		{
			name:           "hide summaries only",
			degradation:    Degradation{HideListSummaries: true},
			wantCompact:    true,
			wantShowDesc:   true,
			wantShowTimer:  true,
		},
		{
			name:           "hide descriptions only",
			degradation:    Degradation{HideListDescriptions: true},
			wantCompact:    true,
			wantShowDesc:   false,
			wantShowTimer:  true,
		},
		{
			name:           "hide timer only",
			degradation:    Degradation{HideTimerInfo: true},
			wantCompact:    false,
			wantShowDesc:   true,
			wantShowTimer:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantCompact, tt.degradation.IsCompactMode(), "IsCompactMode")
			assert.Equal(t, tt.wantShowDesc, tt.degradation.ShouldShowDescription(), "ShouldShowDescription")
			assert.Equal(t, tt.wantShowTimer, tt.degradation.ShouldShowTimer(), "ShouldShowTimer")
		})
	}
}

func TestLayoutModeString(t *testing.T) {
	tests := []struct {
		mode LayoutMode
		want string
	}{
		{LayoutFull, "full"},
		{LayoutStandard, "standard"},
		{LayoutCompact, "compact"},
		{LayoutMinimal, "minimal"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.mode.String())
		})
	}
}

func TestComputeOverlaySize(t *testing.T) {
	tests := []struct {
		name       string
		termWidth  int
		termHeight int
		maxWidth   int
		maxHeight  int
		wantWidth  int
		wantHeight int
	}{
		{
			name:       "large terminal - uses max",
			termWidth:  150,
			termHeight: 50,
			maxWidth:   80,
			maxHeight:  25,
			wantWidth:  80,
			wantHeight: 25,
		},
		{
			name:       "small terminal - constrained by margin and min",
			termWidth:  60,
			termHeight: 20,
			maxWidth:   80,
			maxHeight:  25,
			wantWidth:  52,  // min(60-8, 80) = min(52, 80) = 52, clamped to [40, 52]
			wantHeight: 12,  // min(20-8, 25) = min(12, 25) = 12, clamped to [10, 12]
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := ComputeOverlaySize(tt.termWidth, tt.termHeight, tt.maxWidth, tt.maxHeight)
			assert.Equal(t, tt.wantWidth, w, "width")
			assert.Equal(t, tt.wantHeight, h, "height")
		})
	}
}

// TestResponsiveBreakpoints verifies the breakpoint thresholds are sensible
func TestResponsiveBreakpoints(t *testing.T) {
	// Verify thresholds are in expected order
	assert.Less(t, MinWidth, CompactWidth)
	assert.Less(t, CompactWidth, StandardWidth)
	assert.Less(t, StandardWidth, FullWidth)

	assert.Less(t, MinHeight, CompactHeight)
	assert.Less(t, CompactHeight, StandardHeight)
	assert.Less(t, StandardHeight, FullHeight)

	// Verify minimum dimensions are reasonable for TUI
	assert.GreaterOrEqual(t, MinWidth, 80, "MinWidth should be at least 80 (standard terminal)")
	assert.GreaterOrEqual(t, MinHeight, 24, "MinHeight should be at least 24 (standard terminal)")
}
