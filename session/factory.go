package session

import (
	"claude-squad/session/zellij"
)

// MultiplexerType represents the type of terminal multiplexer to use.
// Deprecated: Only Zellij is now supported. This type is kept for backwards compatibility.
type MultiplexerType string

const (
	// MultiplexerZellij is the only supported multiplexer.
	MultiplexerZellij MultiplexerType = "zellij"
)

// NewMultiplexer creates a new Zellij session.
// The mtype parameter is deprecated and ignored (kept for backwards compatibility).
func NewMultiplexer(mtype MultiplexerType, name, program string) Multiplexer {
	return zellij.NewZellijSession(name, program)
}

// IsMultiplexerAvailable checks if Zellij is available on the system.
func IsMultiplexerAvailable() bool {
	return zellij.IsAvailable()
}
