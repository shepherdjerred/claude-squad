package session

// Multiplexer defines the interface for terminal multiplexer sessions.
// This abstraction allows claude-squad to use Zellij for terminal session management.
type Multiplexer interface {
	// Start creates and starts a new session with the given working directory.
	// The program to run is configured during construction.
	Start(workDir string) error

	// Restore attaches to an existing session and restores the window size.
	Restore() error

	// Attach attaches to the session for interactive use.
	// Returns a channel that is closed when the session is detached.
	Attach() (chan struct{}, error)

	// Detach disconnects from the current session.
	// Panics if detaching fails. Must be called while attached.
	Detach()

	// DetachSafely disconnects from the current session without panicking.
	DetachSafely() error

	// Close terminates the session and cleans up resources.
	Close() error

	// SendKeys sends keystrokes to the session.
	SendKeys(keys string) error

	// TapEnter sends an enter keystroke to the session.
	TapEnter() error

	// TapDAndEnter sends 'D' followed by enter (for Aider/Gemini).
	TapDAndEnter() error

	// CapturePaneContent captures the current visible content of the pane.
	CapturePaneContent() (string, error)

	// CapturePaneContentWithOptions captures pane content with scroll history.
	// start and end specify line numbers (use "-" for start/end of history).
	CapturePaneContentWithOptions(start, end string) (string, error)

	// HasUpdated checks if pane content has changed since the last check.
	// Returns (updated, hasPrompt) where hasPrompt indicates a user prompt is waiting.
	HasUpdated() (updated bool, hasPrompt bool)

	// DoesSessionExist returns true if the session exists.
	DoesSessionExist() bool

	// SetDetachedSize sets the pane dimensions while detached.
	SetDetachedSize(width, height int) error

	// GetProgram returns the program being run in this session.
	GetProgram() string

	// IsProgramRunning checks if the configured program is actively running in the session.
	// Returns true if the program appears to be running, false if we see a shell prompt
	// or other indicators that the program has exited.
	// This is used to detect when the program needs to be restarted after system reboot.
	IsProgramRunning() (bool, error)

	// RestartProgram restarts the program in the existing session with optional arguments.
	// This sends the program command followed by the args to the terminal and executes it.
	RestartProgram(args string) error
}
