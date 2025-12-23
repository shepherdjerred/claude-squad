package session

import (
	"claude-squad/session/tmux"
	"claude-squad/session/zellij"
	"runtime"
)

// MultiplexerType represents the type of terminal multiplexer to use.
type MultiplexerType string

const (
	MultiplexerTmux   MultiplexerType = "tmux"
	MultiplexerZellij MultiplexerType = "zellij"
)

// DefaultMultiplexer returns the preferred multiplexer for the current platform.
// Zellij is preferred on Unix systems, tmux on Windows (since Zellij doesn't support Windows).
func DefaultMultiplexer() MultiplexerType {
	if runtime.GOOS == "windows" {
		return MultiplexerTmux
	}
	return MultiplexerZellij
}

// NewMultiplexer creates a new session using the specified multiplexer type.
func NewMultiplexer(mtype MultiplexerType, name, program string) Multiplexer {
	switch mtype {
	case MultiplexerZellij:
		return zellij.NewZellijSession(name, program)
	case MultiplexerTmux:
		fallthrough
	default:
		return tmux.NewTmuxSession(name, program)
	}
}

// IsMultiplexerAvailable checks if a multiplexer is available on the system.
func IsMultiplexerAvailable(mtype MultiplexerType) bool {
	switch mtype {
	case MultiplexerZellij:
		return zellij.IsAvailable()
	case MultiplexerTmux:
		return tmux.IsAvailable()
	default:
		return false
	}
}
