// Package harness provides test utilities for Bubble Tea models.
// It wraps models and provides methods for simulating user input.
package harness

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Harness wraps a tea.Model for testing
type Harness struct {
	t      *testing.T
	model  tea.Model
	width  int
	height int
}

// New creates a new Harness for testing the given model
func New(t *testing.T, model tea.Model, width, height int) *Harness {
	h := &Harness{
		t:      t,
		model:  model,
		width:  width,
		height: height,
	}
	// Initialize with window size
	h.SendMsg(tea.WindowSizeMsg{Width: width, Height: height})
	return h
}

// SendMsg sends a tea.Msg to the model and updates it
func (h *Harness) SendMsg(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	h.model, cmd = h.model.Update(msg)
	return cmd
}

// SendKey sends a key press message
func (h *Harness) SendKey(key string) tea.Cmd {
	return h.SendMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

// SendSpecialKey sends a special key (Enter, Tab, etc.)
func (h *Harness) SendSpecialKey(keyType tea.KeyType) tea.Cmd {
	return h.SendMsg(tea.KeyMsg{Type: keyType})
}

// Resize simulates a terminal resize
func (h *Harness) Resize(width, height int) tea.Cmd {
	h.width = width
	h.height = height
	return h.SendMsg(tea.WindowSizeMsg{Width: width, Height: height})
}

// View returns the current rendered view
func (h *Harness) View() string {
	return h.model.View()
}

// Model returns the underlying model (for type assertions)
func (h *Harness) Model() tea.Model {
	return h.model
}

// Width returns the current width
func (h *Harness) Width() int {
	return h.width
}

// Height returns the current height
func (h *Harness) Height() int {
	return h.height
}

// CommonSizes contains common terminal sizes for testing
var CommonSizes = []TerminalSize{
	{Name: "minimum", Width: 80, Height: 24},
	{Name: "compact", Width: 100, Height: 30},
	{Name: "standard", Width: 120, Height: 40},
	{Name: "large", Width: 200, Height: 50},
	{Name: "wide", Width: 200, Height: 24},
	{Name: "tall", Width: 80, Height: 60},
}

// TerminalSize represents a terminal size for testing
type TerminalSize struct {
	Name   string
	Width  int
	Height int
}

// RunWithSizes runs a test function for each terminal size
func RunWithSizes(t *testing.T, sizes []TerminalSize, fn func(t *testing.T, size TerminalSize)) {
	for _, size := range sizes {
		t.Run(size.Name, func(t *testing.T) {
			fn(t, size)
		})
	}
}

// RunWithCommonSizes runs a test function for all common terminal sizes
func RunWithCommonSizes(t *testing.T, fn func(t *testing.T, size TerminalSize)) {
	RunWithSizes(t, CommonSizes, fn)
}

// KeySequence represents a sequence of key presses
type KeySequence []tea.Msg

// NewKeySequence creates a key sequence from string input
func NewKeySequence(keys ...string) KeySequence {
	var seq KeySequence
	for _, key := range keys {
		seq = append(seq, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	}
	return seq
}

// Play sends all messages in the sequence to the harness
func (seq KeySequence) Play(h *Harness) {
	for _, msg := range seq {
		h.SendMsg(msg)
	}
}
