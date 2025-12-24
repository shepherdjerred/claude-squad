package zellij

import (
	"bytes"
	"claude-squad/cmd"
	"claude-squad/log"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

const (
	ProgramClaude = "claude"
	ProgramAider  = "aider"
	ProgramGemini = "gemini"
)

const ZellijPrefix = "claudesquad_"

const (
	captureFileMaxRetries   = 3
	captureFileInitialDelay = 5 * time.Millisecond
	captureFileMaxDelay     = 100 * time.Millisecond
)

var whiteSpaceRegex = regexp.MustCompile(`\s+`)
var ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func toClaudeSquadZellijName(str string) string {
	str = whiteSpaceRegex.ReplaceAllString(str, "")
	str = strings.ReplaceAll(str, ".", "_")
	return fmt.Sprintf("%s%s", ZellijPrefix, str)
}

// ZellijSession represents a managed Zellij session.
type ZellijSession struct {
	// Initialized by NewZellijSession
	sanitizedName string
	program       string
	cmdExec       cmd.Executor

	// Initialized by Start or Restore
	ptmx    *os.File
	monitor *statusMonitor

	// Content cache for performance
	contentCache *contentCache

	// Terminal buffer for capturing PTY output with colors
	termBuffer   *TerminalBuffer
	ptyReaderCtx context.Context
	ptyReaderCancel context.CancelFunc

	// Initialized by Attach, deinitialized by Detach
	attachCh chan struct{}
	ctx      context.Context
	cancel   func()
	wg       *sync.WaitGroup
}

// NewZellijSession creates a new ZellijSession with the given name and program.
func NewZellijSession(name string, program string) *ZellijSession {
	return newZellijSession(name, program, cmd.MakeExecutor())
}

// NewZellijSessionWithDeps creates a new ZellijSession with provided dependencies for testing.
func NewZellijSessionWithDeps(name string, program string, cmdExec cmd.Executor) *ZellijSession {
	return newZellijSession(name, program, cmdExec)
}

func newZellijSession(name string, program string, cmdExec cmd.Executor) *ZellijSession {
	return &ZellijSession{
		sanitizedName: toClaudeSquadZellijName(name),
		program:       program,
		cmdExec:       cmdExec,
		contentCache:  newContentCache(200 * time.Millisecond),
		termBuffer:    NewTerminalBuffer(),
	}
}

// Start creates and starts a new Zellij session.
func (z *ZellijSession) Start(workDir string) error {
	if z.DoesSessionExist() {
		return fmt.Errorf("zellij session already exists: %s", z.sanitizedName)
	}

	// Create a temporary layout file that runs the program
	// KDL format for Zellij layouts
	layoutContent := fmt.Sprintf(`layout {
    pane {
        cwd "%s"
        command "sh"
        args "-c" "%s"
    }
}
`, workDir, z.program)

	layoutFile := filepath.Join(os.TempDir(), fmt.Sprintf("zellij_layout_%s.kdl", z.sanitizedName))
	if err := os.WriteFile(layoutFile, []byte(layoutContent), 0644); err != nil {
		return fmt.Errorf("error creating layout file: %w", err)
	}
	// Debug: log the layout file content and path
	log.InfoLog.Printf("Created layout file at %s with content:\n%s", layoutFile, layoutContent)
	defer os.Remove(layoutFile)

	// Create session with the layout
	// Using --new-session-with-layout creates a new session without attaching
	// We run it in the background by spawning and immediately returning
	// Disable startup tips and release notes to speed up session creation
	startCmd := exec.Command("zellij", "--session", z.sanitizedName, "--new-session-with-layout", layoutFile,
		"options", "--attach-to-session", "false", "--show-startup-tips", "false", "--show-release-notes", "false")

	// Clear Zellij environment variables to prevent nesting issues
	// when creating a session from within an existing Zellij session
	startCmd.Env = os.Environ()
	for i := len(startCmd.Env) - 1; i >= 0; i-- {
		if strings.HasPrefix(startCmd.Env[i], "ZELLIJ") {
			startCmd.Env = append(startCmd.Env[:i], startCmd.Env[i+1:]...)
		}
	}

	// Redirect stdin/stdout/stderr to /dev/null to prevent TTY access issues
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return fmt.Errorf("error opening /dev/null: %w", err)
	}
	defer devNull.Close()

	startCmd.Stdin = devNull
	startCmd.Stdout = devNull
	startCmd.Stderr = devNull

	// Start the command but don't wait for it (it would hang waiting for user input)
	if err := startCmd.Start(); err != nil {
		return fmt.Errorf("error creating zellij session: %w", err)
	}

	// Wait for session to exist with exponential backoff
	// No need for initial sleep - the polling loop handles waiting efficiently
	timeout := time.After(5 * time.Second)
	sleepDuration := 10 * time.Millisecond
	for !z.DoesSessionExist() {
		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting for zellij session %s (ensure zellij is installed and working)", z.sanitizedName)
		default:
			time.Sleep(sleepDuration)
			if sleepDuration < 100*time.Millisecond {
				sleepDuration *= 2
			}
		}
	}

	// Now restore (attach PTY) for monitoring
	if err := z.Restore(); err != nil {
		z.Close()
		return fmt.Errorf("error restoring zellij session: %w", err)
	}

	// Handle trust screen in background to avoid blocking session creation
	// This speeds up session creation significantly (from 30-45s to <1s)
	go z.handleTrustScreen()

	return nil
}

// handleTrustScreen handles the "Do you trust the files?" prompt in the background.
// This runs asynchronously to avoid blocking session creation.
func (z *ZellijSession) handleTrustScreen() {
	if !strings.HasSuffix(z.program, ProgramClaude) && !strings.HasSuffix(z.program, ProgramAider) && !strings.HasSuffix(z.program, ProgramGemini) {
		return
	}

	searchString := "Do you trust the files in this folder?"
	tapFunc := z.TapEnter
	maxWaitTime := 30 * time.Second
	if !strings.HasSuffix(z.program, ProgramClaude) {
		searchString = "Open documentation url for more info"
		tapFunc = z.TapDAndEnter
		maxWaitTime = 45 * time.Second
	}

	startTime := time.Now()
	sleepDuration := 100 * time.Millisecond

	for time.Since(startTime) < maxWaitTime {
		time.Sleep(sleepDuration)
		content, err := z.CapturePaneContent()
		if err == nil && strings.Contains(content, searchString) {
			if err := tapFunc(); err != nil {
				log.ErrorLog.Printf("could not tap enter on trust screen: %v", err)
			}
			return
		}
		sleepDuration = time.Duration(float64(sleepDuration) * 1.2)
		if sleepDuration > time.Second {
			sleepDuration = time.Second
		}
	}
}

// Restore sets up the PTY for an existing session.
func (z *ZellijSession) Restore() error {
	// Check if session exists before trying to attach
	if !z.DoesSessionExist() {
		return fmt.Errorf("zellij session does not exist: %s", z.sanitizedName)
	}

	ptmx, err := pty.Start(exec.Command("zellij", "attach", z.sanitizedName))
	if err != nil {
		return fmt.Errorf("error opening PTY: %w", err)
	}
	z.ptmx = ptmx
	z.monitor = newStatusMonitor()

	// Start background PTY reader to capture output with colors
	z.startPTYReader()

	return nil
}

// startPTYReader starts a background goroutine that reads from the PTY
// and feeds the output to the terminal buffer for color capture.
func (z *ZellijSession) startPTYReader() {
	// Stop any existing reader
	if z.ptyReaderCancel != nil {
		z.ptyReaderCancel()
	}

	// Reset the terminal buffer for fresh capture
	if z.termBuffer != nil {
		z.termBuffer.Reset()
	}

	z.ptyReaderCtx, z.ptyReaderCancel = context.WithCancel(context.Background())

	// Capture references locally to avoid race conditions with stopPTYReader
	ctx := z.ptyReaderCtx
	ptmx := z.ptmx
	termBuffer := z.termBuffer

	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// Read from PTY
				n, err := ptmx.Read(buf)
				if err != nil {
					// PTY closed or error, stop reading
					return
				}
				if n > 0 {
					termBuffer.Write(buf[:n])
				}
			}
		}
	}()
}

// stopPTYReader stops the background PTY reader goroutine.
func (z *ZellijSession) stopPTYReader() {
	if z.ptyReaderCancel != nil {
		z.ptyReaderCancel()
		// Don't nil out ptyReaderCtx here - the goroutine may still be accessing it
		// through the closure. The cancel is sufficient to stop the goroutine.
		z.ptyReaderCancel = nil
	}
}

// Attach attaches to the session for interactive use.
func (z *ZellijSession) Attach() (chan struct{}, error) {
	// Stop PTY reader - we'll use direct I/O during attach
	z.stopPTYReader()

	z.attachCh = make(chan struct{})
	z.wg = &sync.WaitGroup{}
	z.wg.Add(1)
	z.ctx, z.cancel = context.WithCancel(context.Background())

	// Resize the PTY to fullscreen BEFORE starting I/O goroutines.
	// This prevents content rendered at preview-pane size from being displayed
	// when we start copying PTY output to stdout.
	cols, rows, err := term.GetSize(int(os.Stdin.Fd()))
	if err == nil {
		_ = z.SetDetachedSize(cols, rows)
	}

	// Show a hint to the user about how to detach
	fmt.Fprintf(os.Stdout, "\033[90m--- Press Ctrl+Q to detach ---\033[0m\n")

	// Goroutine to copy output from PTY to stdout
	go func() {
		defer z.wg.Done()
		_, _ = io.Copy(os.Stdout, z.ptmx)
		select {
		case <-z.ctx.Done():
			// Normal detach
		default:
			fmt.Fprintf(os.Stderr, "\n\033[31mError: Session terminated without detaching. Use Ctrl-Q to properly detach from zellij sessions.\033[0m\n")
		}
	}()

	// Goroutine to read stdin and forward to PTY
	go func() {
		timeoutCh := make(chan struct{})
		go func() {
			time.Sleep(50 * time.Millisecond)
			close(timeoutCh)
		}()

		buf := make([]byte, 32)
		for {
			nr, err := os.Stdin.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				continue
			}

			// Nuke initial control sequences
			select {
			case <-timeoutCh:
			default:
				log.InfoLog.Printf("nuked first stdin: %s", buf[:nr])
				continue
			}

			// Check for Ctrl+q (ASCII 17)
			if nr == 1 && buf[0] == 17 {
				z.Detach()
				return
			}

			_, _ = z.ptmx.Write(buf[:nr])
		}
	}()

	z.monitorWindowSize()
	return z.attachCh, nil
}

// Detach disconnects from the current session.
func (z *ZellijSession) Detach() {
	defer func() {
		close(z.attachCh)
		z.attachCh = nil
		z.cancel = nil
		z.ctx = nil
		z.wg = nil
	}()

	if err := z.ptmx.Close(); err != nil {
		msg := fmt.Sprintf("error closing PTY: %v", err)
		log.ErrorLog.Println(msg)
		panic(msg)
	}

	if err := z.Restore(); err != nil {
		msg := fmt.Sprintf("error restoring after detach: %v", err)
		log.ErrorLog.Println(msg)
		panic(msg)
	}

	z.cancel()
	z.wg.Wait()
}

// DetachSafely disconnects without panicking.
func (z *ZellijSession) DetachSafely() error {
	if z.attachCh == nil {
		return nil
	}

	var errs []error

	// Stop PTY reader before closing PTY
	z.stopPTYReader()

	if z.ptmx != nil {
		if err := z.ptmx.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing PTY: %w", err))
		}
		z.ptmx = nil
	}

	if z.attachCh != nil {
		close(z.attachCh)
		z.attachCh = nil
	}

	if z.cancel != nil {
		z.cancel()
		z.cancel = nil
	}

	if z.wg != nil {
		z.wg.Wait()
		z.wg = nil
	}

	z.ctx = nil

	if len(errs) > 0 {
		return fmt.Errorf("errors during detach: %v", errs)
	}
	return nil
}

// Close terminates the session and cleans up resources.
func (z *ZellijSession) Close() error {
	var errs []error

	// Stop PTY reader before closing PTY
	z.stopPTYReader()

	if z.ptmx != nil {
		if err := z.ptmx.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing PTY: %w", err))
		}
		z.ptmx = nil
	}

	killCmd := exec.Command("zellij", "kill-session", z.sanitizedName)
	if err := z.cmdExec.Run(killCmd); err != nil {
		errs = append(errs, fmt.Errorf("error killing zellij session: %w", err))
	}

	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}

	errMsg := "multiple errors during cleanup:"
	for _, err := range errs {
		errMsg += "\n  - " + err.Error()
	}
	return errors.New(errMsg)
}

// SendKeys sends keystrokes to the session.
func (z *ZellijSession) SendKeys(keys string) error {
	// Invalidate cache when sending keys
	z.contentCache.Invalidate()

	cmd := exec.Command("zellij", "-s", z.sanitizedName, "action", "write-chars", keys)
	return z.cmdExec.Run(cmd)
}

// TapEnter sends an enter keystroke.
func (z *ZellijSession) TapEnter() error {
	z.contentCache.Invalidate()
	// Send carriage return (byte 13)
	cmd := exec.Command("zellij", "-s", z.sanitizedName, "action", "write", "13")
	return z.cmdExec.Run(cmd)
}

// TapDAndEnter sends 'D' followed by enter (for Aider/Gemini).
func (z *ZellijSession) TapDAndEnter() error {
	z.contentCache.Invalidate()
	// Send 'D' then carriage return
	if err := z.SendKeys("D"); err != nil {
		return err
	}
	return z.TapEnter()
}

// readCaptureFileWithRetry attempts to read the capture file with retries.
// It waits for the file to exist and have content before reading.
func readCaptureFileWithRetry(filePath string) ([]byte, error) {
	var lastErr error
	delay := captureFileInitialDelay

	for attempt := 0; attempt <= captureFileMaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(delay)
			delay = time.Duration(float64(delay) * 1.5)
			if delay > captureFileMaxDelay {
				delay = captureFileMaxDelay
			}
		}

		// Check if file exists and has content
		info, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				lastErr = fmt.Errorf("capture file does not exist (attempt %d/%d): %w", attempt+1, captureFileMaxRetries+1, err)
				continue
			}
			lastErr = fmt.Errorf("error checking capture file (attempt %d/%d): %w", attempt+1, captureFileMaxRetries+1, err)
			continue
		}

		// Verify file has content (non-empty)
		if info.Size() == 0 {
			lastErr = fmt.Errorf("capture file is empty (attempt %d/%d)", attempt+1, captureFileMaxRetries+1)
			continue
		}

		// Read the file
		content, err := os.ReadFile(filePath)
		if err != nil {
			lastErr = fmt.Errorf("error reading capture file (attempt %d/%d): %w", attempt+1, captureFileMaxRetries+1, err)
			continue
		}

		// Verify content was read
		if len(content) == 0 {
			lastErr = fmt.Errorf("capture file read returned empty content (attempt %d/%d)", attempt+1, captureFileMaxRetries+1)
			continue
		}

		return content, nil
	}

	return nil, fmt.Errorf("failed to read capture file after %d attempts: %w", captureFileMaxRetries+1, lastErr)
}

// CapturePaneContent captures the current pane content.
// Uses the terminal buffer for colored output when available.
func (z *ZellijSession) CapturePaneContent() (string, error) {
	// Check cache first
	if content, _, valid := z.contentCache.Get(); valid {
		return content, nil
	}

	// Try to get content from terminal buffer (with colors)
	if z.termBuffer != nil {
		rendered := z.termBuffer.Render()
		if len(strings.TrimSpace(rendered)) > 0 {
			z.contentCache.Set(rendered)
			return rendered, nil
		}
	}

	// Fall back to dump-screen (no colors, but works during startup)
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("zellij_capture_%s_%d.txt", z.sanitizedName, time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	cmd := exec.Command("zellij", "-s", z.sanitizedName, "action", "dump-screen", tmpFile)
	if err := z.cmdExec.Run(cmd); err != nil {
		return "", fmt.Errorf("error capturing pane content: %w", err)
	}

	content, err := readCaptureFileWithRetry(tmpFile)
	if err != nil {
		return "", fmt.Errorf("error reading capture file for session %s: %w", z.sanitizedName, err)
	}

	result := string(content)
	z.contentCache.Set(result)
	return result, nil
}

// CapturePaneContentWithOptions captures pane content with scroll history.
func (z *ZellijSession) CapturePaneContentWithOptions(start, end string) (string, error) {
	// For full history, use the -f/--full flag
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("zellij_capture_%s_%d.txt", z.sanitizedName, time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	args := []string{"-s", z.sanitizedName, "action", "dump-screen"}
	if start == "-" && end == "-" {
		args = append(args, "--full")
	}
	args = append(args, tmpFile)

	cmd := exec.Command("zellij", args...)
	if err := z.cmdExec.Run(cmd); err != nil {
		return "", fmt.Errorf("error capturing pane content: %w", err)
	}

	content, err := readCaptureFileWithRetry(tmpFile)
	if err != nil {
		return "", fmt.Errorf("error reading capture file for session %s: %w", z.sanitizedName, err)
	}

	return string(content), nil
}

// HasUpdated checks if pane content has changed since the last check.
func (z *ZellijSession) HasUpdated() (updated bool, hasPrompt bool) {
	content, err := z.CapturePaneContent()
	if err != nil {
		log.ErrorLog.Printf("error capturing pane content: %v", err)
		return false, false
	}

	// Check for prompts based on program type
	if z.program == ProgramClaude {
		hasPrompt = strings.Contains(content, "No, and tell Claude what to do differently")
	} else if strings.HasPrefix(z.program, ProgramAider) {
		hasPrompt = strings.Contains(content, "(Y)es/(N)o/(D)on't ask again")
	} else if strings.HasPrefix(z.program, ProgramGemini) {
		hasPrompt = strings.Contains(content, "Yes, allow once")
	}

	newHash := z.monitor.hash(content)
	if !bytes.Equal(newHash, z.monitor.prevOutputHash) {
		z.monitor.prevOutputHash = newHash
		return true, hasPrompt
	}
	return false, hasPrompt
}

// DoesSessionExist returns true if the session exists.
func (z *ZellijSession) DoesSessionExist() bool {
	cmd := exec.Command("zellij", "list-sessions")
	output, err := z.cmdExec.Output(cmd)
	if err != nil {
		return false
	}

	// Strip ANSI escape codes from output (zellij uses colors in list-sessions)
	cleanOutput := ansiEscapeRegex.ReplaceAllString(string(output), "")

	sessions := strings.Split(cleanOutput, "\n")
	for _, session := range sessions {
		// Session names may have additional info, get just the name
		name := strings.Fields(session)
		if len(name) > 0 && name[0] == z.sanitizedName {
			return true
		}
	}
	return false
}

// SetDetachedSize sets the pane dimensions while detached.
func (z *ZellijSession) SetDetachedSize(width, height int) error {
	// Also resize the terminal buffer
	if z.termBuffer != nil {
		z.termBuffer.Resize(height, width)
	}

	if z.ptmx == nil {
		return nil
	}
	return pty.Setsize(z.ptmx, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
		X:    0,
		Y:    0,
	})
}

// GetProgram returns the program being run in this session.
func (z *ZellijSession) GetProgram() string {
	return z.program
}

// IsProgramRunning checks if the configured program is actively running in the session.
// Returns true if the program appears to be running, false if we see a shell prompt
// or other indicators that the program has exited.
func (z *ZellijSession) IsProgramRunning() (bool, error) {
	content, err := z.CapturePaneContent()
	if err != nil {
		return false, fmt.Errorf("failed to capture pane content: %w", err)
	}

	return detectProgramRunning(content, z.program), nil
}

// detectProgramRunning analyzes terminal content to determine if a program is running.
func detectProgramRunning(content, program string) bool {
	contentLen := len(strings.TrimSpace(content))

	// If content is empty or very short, assume program is not running
	if contentLen < 10 {
		log.DebugLog.Printf("[detectProgramRunning] Content too short (%d chars), assuming not running", contentLen)
		return false
	}

	// Check for program-specific indicators that it IS running
	programRunningIndicators := []string{
		"Do you trust the files",      // Claude trust prompt
		"Claude Code",                  // Claude startup
		"No, and tell Claude",          // Claude permission prompt
		"(Y)es/(N)o/(D)on't ask again", // Aider prompt
		"Yes, allow once",              // Gemini prompt
		"Open documentation url",       // Aider startup
	}

	for _, indicator := range programRunningIndicators {
		if strings.Contains(content, indicator) {
			log.DebugLog.Printf("[detectProgramRunning] Found program indicator '%s', program is running", indicator)
			return true
		}
	}

	// Get the last few non-empty lines to check for shell prompts
	lines := strings.Split(content, "\n")
	var lastLines []string
	for i := len(lines) - 1; i >= 0 && len(lastLines) < 5; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			lastLines = append(lastLines, line)
		}
	}

	// Shell prompt patterns that indicate the program is NOT running
	shellPromptPatterns := []string{
		"$ ",           // bash prompt
		"% ",           // zsh prompt
		"# ",           // root prompt
		"❯ ",           // starship/fancy prompts
		"➜ ",           // oh-my-zsh
		"[exited]",     // process exited indicator
		"[Exited]",     // process exited indicator
		"exited with",  // exit message
	}

	// Check if any of the last lines contain shell prompt patterns
	for _, line := range lastLines {
		// Check for explicit shell prompts
		for _, pattern := range shellPromptPatterns {
			if strings.Contains(line, pattern) {
				log.DebugLog.Printf("[detectProgramRunning] Found shell prompt pattern '%s' in line '%s', program NOT running", pattern, line)
				return false
			}
		}

		// Check for user@host:path$ pattern (common shell prompt)
		if strings.Contains(line, "@") && (strings.HasSuffix(line, "$ ") || strings.HasSuffix(line, "# ") || strings.HasSuffix(line, "% ")) {
			log.DebugLog.Printf("[detectProgramRunning] Found user@host shell prompt in line '%s', program NOT running", line)
			return false
		}
	}

	// If we didn't find explicit indicators either way, assume program is running
	// This is a conservative default to avoid false restarts
	log.DebugLog.Printf("[detectProgramRunning] No definitive indicators found, assuming program is running (conservative)")
	return true
}

// RestartProgram restarts the program in the existing session with optional arguments.
// This sends the program command to the terminal and executes it.
func (z *ZellijSession) RestartProgram(args string) error {
	// Build the command string
	command := z.program
	if args != "" {
		command = command + " " + args
	}

	// Send the command to the terminal
	if err := z.SendKeys(command); err != nil {
		return fmt.Errorf("failed to send program command: %w", err)
	}

	// Execute the command
	if err := z.TapEnter(); err != nil {
		return fmt.Errorf("failed to execute program command: %w", err)
	}

	log.InfoLog.Printf("Restarted program in session %s: %s", z.sanitizedName, command)

	// Handle trust screen in background
	go z.handleTrustScreen()

	return nil
}

// IsAvailable checks if Zellij is available on the system.
func IsAvailable() bool {
	cmd := exec.Command("zellij", "--version")
	return cmd.Run() == nil
}

// CleanupSessions kills all Zellij sessions that start with the claude-squad prefix.
func CleanupSessions(cmdExec cmd.Executor) error {
	cmd := exec.Command("zellij", "list-sessions")
	output, err := cmdExec.Output(cmd)
	if err != nil {
		// No sessions or zellij not running
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("failed to list zellij sessions: %w", err)
	}

	// Strip ANSI escape codes from output (zellij uses colors in list-sessions)
	cleanOutput := ansiEscapeRegex.ReplaceAllString(string(output), "")

	sessions := strings.Split(cleanOutput, "\n")
	for _, session := range sessions {
		name := strings.Fields(session)
		if len(name) > 0 && strings.HasPrefix(name[0], ZellijPrefix) {
			log.InfoLog.Printf("cleaning up zellij session: %s", name[0])
			if err := cmdExec.Run(exec.Command("zellij", "kill-session", name[0])); err != nil {
				return fmt.Errorf("failed to kill zellij session %s: %w", name[0], err)
			}
		}
	}
	return nil
}

// statusMonitor monitors pane content for changes.
type statusMonitor struct {
	prevOutputHash []byte
}

func newStatusMonitor() *statusMonitor {
	return &statusMonitor{}
}

// hash computes SHA256 without allocating a byte slice from the string.
func (m *statusMonitor) hash(s string) []byte {
	h := sha256.New()
	io.WriteString(h, s)
	return h.Sum(nil)
}
