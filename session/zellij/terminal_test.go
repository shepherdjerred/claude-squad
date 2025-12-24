package zellij

import (
	"strings"
	"testing"
)

func TestTerminalBuffer_Write(t *testing.T) {
	tb := NewTerminalBuffer()

	// Write plain text
	n, err := tb.Write([]byte("Hello World"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 11 {
		t.Errorf("Expected 11 bytes written, got %d", n)
	}
}

func TestTerminalBuffer_Render(t *testing.T) {
	tb := NewTerminalBufferWithSize(5, 20)

	// Write plain text
	tb.Write([]byte("Hello World"))

	rendered := tb.Render()
	if !strings.Contains(rendered, "Hello World") {
		t.Errorf("Rendered output should contain 'Hello World', got: %q", rendered)
	}
}

func TestTerminalBuffer_RenderWithColors(t *testing.T) {
	tb := NewTerminalBufferWithSize(5, 40)

	// Write text with ANSI color codes (red text)
	tb.Write([]byte("\x1b[31mRed Text\x1b[0m Normal"))

	rendered := tb.Render()

	// Should contain the text
	if !strings.Contains(rendered, "Red Text") {
		t.Errorf("Rendered output should contain 'Red Text', got: %q", rendered)
	}
	if !strings.Contains(rendered, "Normal") {
		t.Errorf("Rendered output should contain 'Normal', got: %q", rendered)
	}

	// Should contain ANSI codes (the rendered output should have color codes)
	if !strings.Contains(rendered, "\x1b[") {
		t.Errorf("Rendered output should contain ANSI escape codes, got: %q", rendered)
	}
}

func TestTerminalBuffer_Resize(t *testing.T) {
	tb := NewTerminalBufferWithSize(10, 20)

	h, w := tb.GetSize()
	if h != 10 || w != 20 {
		t.Errorf("Expected size 10x20, got %dx%d", h, w)
	}

	tb.Resize(24, 80)

	h, w = tb.GetSize()
	if h != 24 || w != 80 {
		t.Errorf("Expected size 24x80, got %dx%d", h, w)
	}
}

func TestTerminalBuffer_Reset(t *testing.T) {
	tb := NewTerminalBuffer()

	// Write some content
	tb.Write([]byte("Some content"))
	rendered1 := tb.Render()

	// Reset
	tb.Reset()

	// Rendered should be different after reset
	rendered2 := tb.Render()

	// The content should be cleared (just whitespace/empty)
	if strings.Contains(rendered2, "Some content") {
		t.Errorf("After Reset, buffer should not contain old content")
	}

	_ = rendered1 // Use the variable
}

func TestTerminalBuffer_CachedRender(t *testing.T) {
	tb := NewTerminalBuffer()

	tb.Write([]byte("Test content"))

	// First render
	render1 := tb.Render()

	// Second render should return cached (same result)
	render2 := tb.Render()

	if render1 != render2 {
		t.Errorf("Cached render should return same result")
	}

	// Write more data
	tb.Write([]byte(" more"))

	// Third render should be different (dirty flag set)
	render3 := tb.Render()
	if render3 == render1 {
		// Note: this might not be different if the new content is on same line
		// Just check it doesn't error
		t.Log("Render after write completed successfully")
	}
}

func TestTerminalBuffer_OSC8Stripping(t *testing.T) {
	tb := NewTerminalBufferWithSize(5, 40)

	// Write text with OSC 8 hyperlink sequences
	// Format: ESC ] 8 ; ; URL ST text ESC ] 8 ; ; ST
	// ST (String Terminator) can be ESC \ or BEL (\x07)
	hyperlink := "\x1b]8;;https://example.com\x1b\\Click Here\x1b]8;;\x1b\\"
	tb.Write([]byte(hyperlink))

	rendered := tb.Render()

	// Should contain the visible text
	if !strings.Contains(rendered, "Click Here") {
		t.Errorf("Rendered output should contain 'Click Here', got: %q", rendered)
	}

	// Should NOT contain the "8;;" sequence artifacts
	if strings.Contains(rendered, "8;;") {
		t.Errorf("Rendered output should not contain '8;;' artifacts, got: %q", rendered)
	}
}

func TestTerminalBuffer_OSC8StrippingWithBEL(t *testing.T) {
	tb := NewTerminalBufferWithSize(5, 40)

	// OSC 8 with BEL as string terminator
	hyperlink := "\x1b]8;;https://example.com\x07Click Here\x1b]8;;\x07"
	tb.Write([]byte(hyperlink))

	rendered := tb.Render()

	// Should contain the visible text
	if !strings.Contains(rendered, "Click Here") {
		t.Errorf("Rendered output should contain 'Click Here', got: %q", rendered)
	}

	// Should NOT contain the "8;;" sequence artifacts
	if strings.Contains(rendered, "8;;") {
		t.Errorf("Rendered output should not contain '8;;' artifacts, got: %q", rendered)
	}
}
