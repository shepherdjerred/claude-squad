package session

import (
	"claude-squad/config"
	"claude-squad/session/docker"
	"claude-squad/session/zellij"
)

// MultiplexerType represents the type of terminal multiplexer to use.
// Deprecated: Only Zellij is now supported. This type is kept for backwards compatibility.
type MultiplexerType string

const (
	// MultiplexerZellij is the only supported multiplexer.
	MultiplexerZellij MultiplexerType = "zellij"
)

// MultiplexerOptions contains options for creating a multiplexer session.
type MultiplexerOptions struct {
	BaseImage  string
	RepoURL    string
	BranchName string
	WorkDir    string
}

// NewMultiplexer creates a new session based on the session type.
// For backwards compatibility, if sessionType is empty, it defaults to Zellij.
func NewMultiplexer(sessionType string, name, program string, opts MultiplexerOptions) Multiplexer {
	switch sessionType {
	case config.SessionTypeDockerBind, config.SessionTypeDockerClone:
		return docker.NewDockerSession(name, program, sessionType, docker.MultiplexerOptions{
			BaseImage:  opts.BaseImage,
			RepoURL:    opts.RepoURL,
			BranchName: opts.BranchName,
			WorkDir:    opts.WorkDir,
		})
	default:
		return zellij.NewZellijSession(name, program)
	}
}

// NewMultiplexerLegacy creates a new Zellij session for backwards compatibility.
// Deprecated: Use NewMultiplexer with session type instead.
func NewMultiplexerLegacy(mtype MultiplexerType, name, program string) Multiplexer {
	return zellij.NewZellijSession(name, program)
}

// IsMultiplexerAvailable checks if the specified session type is available on the system.
func IsMultiplexerAvailable(sessionType string) bool {
	switch sessionType {
	case config.SessionTypeDockerBind, config.SessionTypeDockerClone:
		return docker.IsDockerAvailable()
	default:
		return zellij.IsAvailable()
	}
}

// IsZellijAvailable checks if Zellij is available on the system.
func IsZellijAvailable() bool {
	return zellij.IsAvailable()
}

// IsDockerAvailable checks if Docker is available on the system.
func IsDockerAvailable() bool {
	return docker.IsDockerAvailable()
}
