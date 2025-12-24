package docker

import (
	"claude-squad/config"
	"claude-squad/log"
	"claude-squad/session/zellij"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

const (
	DockerPrefix      = "claudesquad_"
	containerWorkDir  = "/workspace"
	claudeConfigMount = "/root/.claude"
)

var whiteSpaceRegex = regexp.MustCompile(`\s+`)

func toDockerContainerName(str string) string {
	str = whiteSpaceRegex.ReplaceAllString(str, "")
	str = strings.ReplaceAll(str, ".", "_")
	// Add timestamp suffix for uniqueness
	return fmt.Sprintf("%s%s_%x", DockerPrefix, str, time.Now().Unix()&0xFFFF)
}

// DockerSession represents a managed Docker container session.
type DockerSession struct {
	// Initialized by NewDockerSession
	containerName string
	baseImage     string
	program       string
	sessionType   string // "docker-bind" or "docker-clone"

	// Git info for clone mode
	repoURL    string
	branchName string

	// Host paths
	hostWorkDir   string
	hostClaudeDir string

	// PTY management
	ptmx    *os.File
	execCmd *exec.Cmd

	// Terminal buffer for capturing output with colors
	termBuffer      *zellij.TerminalBuffer
	ptyReaderCtx    context.Context
	ptyReaderCancel context.CancelFunc

	// Content cache for performance
	contentCache *contentCache
	monitor      *statusMonitor

	// Attach state
	attachCh chan struct{}
	ctx      context.Context
	cancel   func()
	wg       *sync.WaitGroup
}

// MultiplexerOptions contains options for creating a multiplexer session.
type MultiplexerOptions struct {
	BaseImage  string
	RepoURL    string
	BranchName string
	WorkDir    string
}

// NewDockerSession creates a new DockerSession with the given parameters.
func NewDockerSession(name, program, sessionType string, opts MultiplexerOptions) *DockerSession {
	claudeDir, _ := os.UserHomeDir()
	claudeDir = claudeDir + "/.claude"

	containerName := name
	if !strings.HasPrefix(name, DockerPrefix) {
		containerName = toDockerContainerName(name)
	}

	return &DockerSession{
		containerName: containerName,
		baseImage:     opts.BaseImage,
		program:       program,
		sessionType:   sessionType,
		repoURL:       opts.RepoURL,
		branchName:    opts.BranchName,
		hostWorkDir:   opts.WorkDir,
		hostClaudeDir: claudeDir,
		termBuffer:    zellij.NewTerminalBuffer(),
		contentCache:  newContentCache(200 * time.Millisecond),
	}
}

// IsDockerAvailable checks if Docker is available and running.
func IsDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// Start creates and starts a new Docker container session.
func (d *DockerSession) Start(workDir string) error {
	d.hostWorkDir = workDir

	if d.DoesSessionExist() {
		return fmt.Errorf("docker container already exists: %s", d.containerName)
	}

	// Build docker run arguments
	args := []string{"run", "-d", "--name", d.containerName}

	// Mount ~/.claude for persistent Claude config
	args = append(args, "-v", fmt.Sprintf("%s:%s", d.hostClaudeDir, claudeConfigMount))

	if d.sessionType == config.SessionTypeDockerBind {
		// Bind-mount mode: mount the worktree
		args = append(args, "-v", fmt.Sprintf("%s:%s", workDir, containerWorkDir))
		args = append(args, "-w", containerWorkDir)
	}

	// Use sleep infinity as entrypoint so container stays running
	args = append(args, d.baseImage, "sleep", "infinity")

	log.InfoLog.Printf("Creating Docker container: docker %s", strings.Join(args, " "))

	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create docker container: %w, output: %s", err, string(output))
	}

	// For clone mode, clone the repository inside the container
	if d.sessionType == config.SessionTypeDockerClone && d.repoURL != "" {
		if err := d.cloneRepoInContainer(); err != nil {
			d.Close()
			return fmt.Errorf("failed to clone repo in container: %w", err)
		}
	}

	// Initialize monitor
	d.monitor = newStatusMonitor()

	// Restore PTY connection
	return d.Restore()
}

// cloneRepoInContainer clones the git repository inside the container.
func (d *DockerSession) cloneRepoInContainer() error {
	// Clone the repo
	cloneCmd := exec.Command("docker", "exec", d.containerName,
		"git", "clone", d.repoURL, containerWorkDir)
	if output, err := cloneCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone failed: %w, output: %s", err, string(output))
	}

	// Create/checkout the branch
	if d.branchName != "" {
		branchCmd := exec.Command("docker", "exec", "-w", containerWorkDir, d.containerName,
			"git", "checkout", "-b", d.branchName)
		if output, err := branchCmd.CombinedOutput(); err != nil {
			// Branch might already exist, try to just checkout
			checkoutCmd := exec.Command("docker", "exec", "-w", containerWorkDir, d.containerName,
				"git", "checkout", d.branchName)
			if output2, err2 := checkoutCmd.CombinedOutput(); err2 != nil {
				return fmt.Errorf("git checkout failed: %w, output: %s %s", err2, string(output), string(output2))
			}
		}
	}

	return nil
}

// Restore attaches to an existing container and restores the PTY.
func (d *DockerSession) Restore() error {
	// Start container if stopped
	if !d.isContainerRunning() {
		startCmd := exec.Command("docker", "start", d.containerName)
		if output, err := startCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to start container: %w, output: %s", err, string(output))
		}
	}

	// Create exec session with PTY
	return d.startExecSession()
}

// isContainerRunning checks if the container is in running state.
func (d *DockerSession) isContainerRunning() bool {
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", d.containerName)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "true"
}

// startExecSession starts a new exec session with Claude.
func (d *DockerSession) startExecSession() error {
	// Build the program command with --dangerously-skip-permissions
	programCmd := d.program
	if strings.Contains(d.program, "claude") && !strings.Contains(d.program, "--dangerously-skip-permissions") {
		programCmd = d.program + " --dangerously-skip-permissions"
	}

	// For clone mode, we need to cd to workspace first
	var execArgs []string
	if d.sessionType == config.SessionTypeDockerClone {
		execArgs = []string{"exec", "-it", "-w", containerWorkDir, d.containerName, "sh", "-c", programCmd}
	} else {
		execArgs = []string{"exec", "-it", d.containerName, "sh", "-c", programCmd}
	}

	d.execCmd = exec.Command("docker", execArgs...)

	// Start with PTY
	ptmx, err := pty.Start(d.execCmd)
	if err != nil {
		return fmt.Errorf("failed to start docker exec with PTY: %w", err)
	}
	d.ptmx = ptmx

	// Start PTY reader for terminal buffer
	d.ptyReaderCtx, d.ptyReaderCancel = context.WithCancel(context.Background())
	go d.readPTYToBuffer()

	// Initialize monitor if not already
	if d.monitor == nil {
		d.monitor = newStatusMonitor()
	}

	return nil
}

// readPTYToBuffer continuously reads from PTY and writes to terminal buffer.
func (d *DockerSession) readPTYToBuffer() {
	buf := make([]byte, 4096)
	for {
		select {
		case <-d.ptyReaderCtx.Done():
			return
		default:
			n, err := d.ptmx.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.ErrorLog.Printf("PTY read error: %v", err)
				}
				return
			}
			if n > 0 {
				d.termBuffer.Write(buf[:n])
				d.monitor.markUpdated()
			}
		}
	}
}

// Attach attaches to the session for interactive use.
func (d *DockerSession) Attach() (chan struct{}, error) {
	d.attachCh = make(chan struct{})
	d.ctx, d.cancel = context.WithCancel(context.Background())
	d.wg = &sync.WaitGroup{}

	// Ensure container is running and we have a PTY
	if d.ptmx == nil {
		if err := d.Restore(); err != nil {
			return nil, err
		}
	}

	// Set terminal to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, fmt.Errorf("failed to set terminal raw mode: %w", err)
	}

	// Get terminal size and resize PTY
	width, height, _ := term.GetSize(int(os.Stdin.Fd()))
	if err := pty.Setsize(d.ptmx, &pty.Winsize{Rows: uint16(height), Cols: uint16(width)}); err != nil {
		log.ErrorLog.Printf("Failed to set PTY size: %v", err)
	}

	// Copy PTY -> stdout
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		io.Copy(os.Stdout, d.ptmx)
	}()

	// Copy stdin -> PTY (with detach detection)
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		buf := make([]byte, 1024)
		for {
			select {
			case <-d.ctx.Done():
				return
			default:
				n, err := os.Stdin.Read(buf)
				if err != nil {
					return
				}
				// Check for Ctrl+Q (ASCII 17) to detach
				for i := 0; i < n; i++ {
					if buf[i] == 17 {
						term.Restore(int(os.Stdin.Fd()), oldState)
						d.cancel()
						close(d.attachCh)
						return
					}
				}
				d.ptmx.Write(buf[:n])
			}
		}
	}()

	// Handle SIGWINCH for terminal resize
	go d.handleResize()

	return d.attachCh, nil
}

// Detach disconnects from the current session.
func (d *DockerSession) Detach() {
	if err := d.DetachSafely(); err != nil {
		panic(fmt.Sprintf("detach failed: %v", err))
	}
}

// DetachSafely disconnects from the current session without panicking.
func (d *DockerSession) DetachSafely() error {
	if d.cancel != nil {
		d.cancel()
	}

	// Close PTY
	if d.ptmx != nil {
		d.ptmx.Close()
		d.ptmx = nil
	}

	// Cancel PTY reader
	if d.ptyReaderCancel != nil {
		d.ptyReaderCancel()
	}

	// Stop the container (preserves filesystem)
	stopCmd := exec.Command("docker", "stop", d.containerName)
	if output, err := stopCmd.CombinedOutput(); err != nil {
		log.ErrorLog.Printf("Failed to stop container: %v, output: %s", err, string(output))
	}

	return nil
}

// Close terminates the session and removes the container.
func (d *DockerSession) Close() error {
	// First detach if attached
	if d.ptmx != nil {
		d.DetachSafely()
	}

	// Remove container forcefully
	rmCmd := exec.Command("docker", "rm", "-f", d.containerName)
	if output, err := rmCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to remove container: %w, output: %s", err, string(output))
	}

	return nil
}

// SendKeys sends keystrokes to the session.
func (d *DockerSession) SendKeys(keys string) error {
	if d.ptmx == nil {
		return fmt.Errorf("not attached to container")
	}
	_, err := d.ptmx.Write([]byte(keys))
	d.contentCache.Invalidate()
	return err
}

// TapEnter sends an enter keystroke to the session.
func (d *DockerSession) TapEnter() error {
	return d.SendKeys("\n")
}

// TapDAndEnter sends 'D' followed by enter (for Aider/Gemini).
func (d *DockerSession) TapDAndEnter() error {
	return d.SendKeys("D\n")
}

// CapturePaneContent captures the current visible content of the pane.
func (d *DockerSession) CapturePaneContent() (string, error) {
	// Check cache first
	if content, _, valid := d.contentCache.Get(); valid {
		return content, nil
	}

	content := d.termBuffer.Render()
	d.contentCache.Set(content)
	return content, nil
}

// CapturePaneContentWithOptions captures pane content with scroll history.
func (d *DockerSession) CapturePaneContentWithOptions(start, end string) (string, error) {
	// Docker session doesn't have scroll history like Zellij
	// Just return current content
	return d.CapturePaneContent()
}

// HasUpdated checks if pane content has changed since the last check.
func (d *DockerSession) HasUpdated() (updated bool, hasPrompt bool) {
	content := d.termBuffer.Render()

	// Check if content changed
	hash := sha256.Sum256([]byte(content))
	updated = d.monitor.hasChanged(hash[:])

	// Check for user prompt
	hasPrompt = d.checkForPrompt(content)

	return updated, hasPrompt
}

// checkForPrompt checks if the content contains a user prompt.
func (d *DockerSession) checkForPrompt(content string) bool {
	// Look for common prompt indicators
	promptIndicators := []string{
		"Do you trust the files",
		"[Y/n]",
		"[y/N]",
		"(yes/no)",
	}
	for _, indicator := range promptIndicators {
		if strings.Contains(content, indicator) {
			return true
		}
	}
	return false
}

// DoesSessionExist returns true if the container exists.
func (d *DockerSession) DoesSessionExist() bool {
	cmd := exec.Command("docker", "inspect", d.containerName)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// SetDetachedSize sets the pane dimensions while detached.
func (d *DockerSession) SetDetachedSize(width, height int) error {
	d.termBuffer.Resize(height, width)
	if d.ptmx != nil {
		return pty.Setsize(d.ptmx, &pty.Winsize{Rows: uint16(height), Cols: uint16(width)})
	}
	return nil
}

// GetProgram returns the program being run in this session.
func (d *DockerSession) GetProgram() string {
	return d.program
}

// IsProgramRunning checks if the configured program is actively running.
func (d *DockerSession) IsProgramRunning() (bool, error) {
	// Check if container is running first
	if !d.isContainerRunning() {
		return false, nil
	}

	// Check if Claude process is running in the container
	psCmd := exec.Command("docker", "exec", d.containerName, "pgrep", "-f", "claude")
	err := psCmd.Run()
	return err == nil, nil
}

// RestartProgram restarts the program in the existing session with optional arguments.
func (d *DockerSession) RestartProgram(args string) error {
	// Close existing PTY session
	if d.ptmx != nil {
		d.ptmx.Close()
		d.ptmx = nil
	}
	if d.ptyReaderCancel != nil {
		d.ptyReaderCancel()
	}

	// Reset terminal buffer
	d.termBuffer.Reset()

	// Start a new exec session
	return d.startExecSession()
}

// GetContainerName returns the container name for this session.
func (d *DockerSession) GetContainerName() string {
	return d.containerName
}
