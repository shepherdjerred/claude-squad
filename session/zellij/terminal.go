package zellij

import (
	"fmt"
	"image/color"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tonistiigi/vt100"
)

const (
	defaultTermWidth  = 80
	defaultTermHeight = 24
)

// oscSequenceRegex matches OSC 8 hyperlink sequences that vt100 doesn't handle.
// Format: ESC ] 8 ; params ; URI ST (where ST is ESC \ or BEL)
var oscSequenceRegex = regexp.MustCompile(`\x1b\]8;[^;]*;[^\x1b\x07]*(?:\x1b\\|\x07)`)

// TerminalBuffer wraps a VT100 terminal emulator to capture PTY output with colors.
// It maintains a cached render of the screen content with ANSI escape codes.
type TerminalBuffer struct {
	mu sync.RWMutex

	vt     *vt100.VT100
	width  int
	height int

	// Cached render output
	cachedRender string
	dirty        bool
	lastRender   time.Time

	// For stopping the background goroutine
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewTerminalBuffer creates a new terminal buffer with default dimensions.
func NewTerminalBuffer() *TerminalBuffer {
	return NewTerminalBufferWithSize(defaultTermHeight, defaultTermWidth)
}

// NewTerminalBufferWithSize creates a new terminal buffer with specified dimensions.
func NewTerminalBufferWithSize(height, width int) *TerminalBuffer {
	return &TerminalBuffer{
		vt:     vt100.NewVT100(height, width),
		width:  width,
		height: height,
		dirty:  true,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Write feeds data to the terminal emulator.
// This is safe to call from multiple goroutines.
func (tb *TerminalBuffer) Write(p []byte) (n int, err error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Strip OSC 8 hyperlink sequences that vt100 doesn't handle
	cleaned := oscSequenceRegex.ReplaceAll(p, nil)

	_, err = tb.vt.Write(cleaned)
	if len(cleaned) > 0 {
		tb.dirty = true
	}
	// Return original length so callers don't see unexpected write lengths
	return len(p), err
}

// Resize changes the terminal dimensions.
func (tb *TerminalBuffer) Resize(height, width int) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if height != tb.height || width != tb.width {
		tb.vt.Resize(height, width)
		tb.height = height
		tb.width = width
		tb.dirty = true
	}
}

// Render returns the current screen content with ANSI escape codes.
// If the buffer hasn't changed since last render, returns cached content.
func (tb *TerminalBuffer) Render() string {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if !tb.dirty && tb.cachedRender != "" {
		return tb.cachedRender
	}

	tb.cachedRender = tb.renderToANSI()
	tb.dirty = false
	tb.lastRender = time.Now()
	return tb.cachedRender
}

// renderToANSI converts the VT100 screen buffer to an ANSI-encoded string.
// Must be called with mu held.
func (tb *TerminalBuffer) renderToANSI() string {
	var sb strings.Builder
	sb.Grow(tb.width * tb.height * 2) // Rough estimate

	var prevFormat vt100.Format
	firstCell := true

	for y := 0; y < tb.height; y++ {
		if y > 0 {
			sb.WriteString("\n")
		}

		// Find the last non-space character in this row to avoid trailing spaces
		lastNonSpace := -1
		for x := tb.width - 1; x >= 0; x-- {
			if tb.vt.Content[y][x] != ' ' && tb.vt.Content[y][x] != 0 {
				lastNonSpace = x
				break
			}
		}

		for x := 0; x <= lastNonSpace || x == 0; x++ {
			char := tb.vt.Content[y][x]
			format := tb.vt.Format[y][x]

			// Emit ANSI codes if format changed
			if firstCell || !formatsEqual(format, prevFormat) {
				ansi := formatToANSI(format)
				if ansi != "" {
					sb.WriteString(ansi)
				}
				prevFormat = format
				firstCell = false
			}

			// Write the character
			if char == 0 {
				sb.WriteRune(' ')
			} else {
				sb.WriteRune(char)
			}
		}
	}

	// Reset formatting at the end
	sb.WriteString("\x1b[0m")

	return sb.String()
}

// formatsEqual compares two Format structs for equality.
func formatsEqual(a, b vt100.Format) bool {
	return a.Fg == b.Fg &&
		a.Bg == b.Bg &&
		a.Intensity == b.Intensity &&
		a.Underscore == b.Underscore &&
		a.Conceal == b.Conceal &&
		a.Negative == b.Negative &&
		a.Blink == b.Blink &&
		a.Inverse == b.Inverse
}

// formatToANSI converts a Format to ANSI escape sequence.
func formatToANSI(f vt100.Format) string {
	var codes []string

	// Reset first
	codes = append(codes, "0")

	// Intensity
	switch f.Intensity {
	case vt100.Bright:
		codes = append(codes, "1")
	case vt100.Dim:
		codes = append(codes, "2")
	}

	// Text attributes
	if f.Underscore {
		codes = append(codes, "4")
	}
	if f.Blink {
		codes = append(codes, "5")
	}
	if f.Inverse {
		codes = append(codes, "7")
	}
	if f.Conceal {
		codes = append(codes, "8")
	}

	// Foreground color
	if fg := colorToANSI(f.Fg, true); fg != "" {
		codes = append(codes, fg)
	}

	// Background color
	if bg := colorToANSI(f.Bg, false); bg != "" {
		codes = append(codes, bg)
	}

	if len(codes) == 1 && codes[0] == "0" {
		// Just reset, which is the default
		return "\x1b[0m"
	}

	return fmt.Sprintf("\x1b[%sm", strings.Join(codes, ";"))
}

// colorToANSI converts a color.RGBA to ANSI color code.
// foreground: true for foreground colors (30-37, 90-97), false for background (40-47, 100-107)
func colorToANSI(c color.RGBA, foreground bool) string {
	// Check if it's the default/zero color
	if c.R == 0 && c.G == 0 && c.B == 0 && c.A == 0 {
		return ""
	}

	// Use 24-bit true color for accurate color representation
	// Modern terminals support this and it avoids incorrect color mapping
	if foreground {
		return fmt.Sprintf("38;2;%d;%d;%d", c.R, c.G, c.B)
	}
	return fmt.Sprintf("48;2;%d;%d;%d", c.R, c.G, c.B)
}

// Reset clears the terminal buffer and creates a fresh VT100 instance.
func (tb *TerminalBuffer) Reset() {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.vt = vt100.NewVT100(tb.height, tb.width)
	tb.cachedRender = ""
	tb.dirty = true
}

// Stop signals the buffer to stop any background processing.
func (tb *TerminalBuffer) Stop() {
	select {
	case <-tb.stopCh:
		// Already stopped
	default:
		close(tb.stopCh)
	}
}

// IsStopped returns true if the buffer has been stopped.
func (tb *TerminalBuffer) IsStopped() bool {
	select {
	case <-tb.stopCh:
		return true
	default:
		return false
	}
}

// GetSize returns the current terminal dimensions.
func (tb *TerminalBuffer) GetSize() (height, width int) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return tb.height, tb.width
}
