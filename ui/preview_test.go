package ui

import (
	"claude-squad/cmd/cmd_test"
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/session/zellij"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// testSetup holds common test setup data
type testSetup struct {
	workdir     string
	instance    *session.Instance
	sessionName string
	cleanupFn   func()
}

// setupTestEnvironment creates a common test environment with git repo and instance
// Note: This does not call Start() because that would require real zellij commands.
// The mock session is set up to handle preview operations directly.
func setupTestEnvironment(t *testing.T, cmdExec cmd_test.MockCmdExec, sessionName string) *testSetup {
	t.Helper()

	// Initialize logging
	log.Initialize(false)

	// Set up a temp working directory
	workdir := t.TempDir()

	// Initialize git repository
	setupGitRepo(t, workdir)

	// Create instance
	instance, err := session.NewInstance(session.InstanceOptions{
		Title:   sessionName,
		Path:    workdir,
		Program: "bash",
		AutoYes: false,
	})
	require.NoError(t, err)

	// Set up zellij session with mocks
	zellijSession := zellij.NewZellijSessionWithDeps(sessionName, "bash", cmdExec)
	instance.SetSession(zellijSession)

	// Note: We don't call instance.Start() because that requires real zellij.
	// The preview tests only need the mock session to be set up.
	// Mark instance as started by setting its internal state.
	instance.MarkAsStartedForTesting()

	// Create cleanup function
	cleanupFn := func() {
		log.Close()
	}

	return &testSetup{
		workdir:     workdir,
		instance:    instance,
		sessionName: sessionName,
		cleanupFn:   cleanupFn,
	}
}

// setupGitRepo initializes a git repository in the given directory
func setupGitRepo(t *testing.T, workdir string) {
	t.Helper()

	// Initialize git repository
	initCmd := exec.Command("git", "init")
	initCmd.Dir = workdir
	err := initCmd.Run()
	require.NoError(t, err)

	// Create basic git config (local to this repo only)
	configCmd := exec.Command("git", "config", "--local", "user.email", "test@example.com")
	configCmd.Dir = workdir
	err = configCmd.Run()
	require.NoError(t, err)

	configCmd = exec.Command("git", "config", "--local", "user.name", "Test User")
	configCmd.Dir = workdir
	err = configCmd.Run()
	require.NoError(t, err)

	// Create and commit a test file
	testFile := filepath.Join(workdir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	addCmd := exec.Command("git", "add", "test.txt")
	addCmd.Dir = workdir
	err = addCmd.Run()
	require.NoError(t, err)

	commitCmd := exec.Command("git", "commit", "-m", "initial commit")
	commitCmd.Dir = workdir
	err = commitCmd.Run()
	require.NoError(t, err)
}

// TestPreviewScrolling tests the scrolling functionality in the preview pane
func TestPreviewScrolling(t *testing.T) {
	// Define session name upfront so mock can return correct value
	sessionName := "test-preview-scroll"

	// Track what commands were executed and their order
	var executedCommands []string
	scrollPosition := 0 // 0 = bottom, positive = scrolled up
	sessionCreated := false

	// Create test content with line numbers for scrolling
	const numLines = 100
	lines := make([]string, numLines+1)
	lines[0] = "$ seq 100" // Command that was run
	for i := 1; i <= numLines; i++ {
		lines[i] = fmt.Sprintf("%d", i)
	}
	fullContent := strings.Join(lines, "\n")

	// Mock command execution
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			cmdStr := cmd.String()
			executedCommands = append(executedCommands, cmdStr)

			// Handle zellij session existence checking
			if strings.Contains(cmdStr, "zellij") && strings.Contains(cmdStr, "list-sessions") {
				if sessionCreated {
					return nil // Session exists
				} else {
					return fmt.Errorf("session does not exist")
				}
			}

			// Handle session creation (zellij attach with layout)
			if strings.Contains(cmdStr, "zellij") && strings.Contains(cmdStr, "attach") {
				sessionCreated = true
				return nil
			}

			// Handle scroll commands
			if strings.Contains(cmdStr, "scroll-up") {
				scrollPosition++
			}
			if strings.Contains(cmdStr, "scroll-down") {
				if scrollPosition > 0 {
					scrollPosition--
				}
			}

			// Handle dump-screen commands - write content to the temp file
			if strings.Contains(cmdStr, "dump-screen") {
				// Get the file path (last argument)
				args := cmd.Args
				if len(args) > 0 {
					tmpFile := args[len(args)-1]
					// Check if this is a full history capture
					if strings.Contains(cmdStr, "--full") {
						os.WriteFile(tmpFile, []byte(fullContent), 0644)
					} else {
						// Regular capture for normal preview mode - show the last 20 lines
						const visibleLines = 20
						startLine := max(0, numLines+1-visibleLines)
						visibleContent := strings.Join(lines[startLine:], "\n")
						os.WriteFile(tmpFile, []byte(visibleContent), 0644)
					}
				}
				return nil
			}

			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			cmdStr := cmd.String()

			// Handle list-sessions for session check - return the correct session name
			if strings.Contains(cmdStr, "list-sessions") {
				if sessionCreated {
					return []byte("claudesquad_" + sessionName), nil
				}
				return []byte(""), fmt.Errorf("no sessions")
			}

			return []byte(""), nil
		},
	}

	// Setup test environment
	setup := setupTestEnvironment(t, cmdExec, sessionName)
	defer setup.cleanupFn()

	// Simulate running a command that produces lots of output
	err := setup.instance.SendKeys("seq 100")
	require.NoError(t, err)
	err = setup.instance.SendKeys("") // Simulate pressing Enter
	require.NoError(t, err)

	// Create the preview pane
	previewPane := NewPreviewPane()
	previewPane.SetSize(80, 30) // Set reasonable size for testing

	// Step 1: Check initial content - should show normal preview mode
	err = previewPane.UpdateContent(setup.instance)
	require.NoError(t, err)

	// Verify we're not in scrolling mode initially
	require.False(t, previewPane.isScrolling, "Should not be in scrolling mode initially")

	// Step 2: Check that PreviewFullHistory returns all content
	fullHistory, err := setup.instance.PreviewFullHistory()
	require.NoError(t, err)

	// Verify that the full history contains both the command and early output
	require.Contains(t, fullHistory, "$ seq 100", "Full history should contain the command")
	require.Contains(t, fullHistory, "1", "Full history should contain earliest output")

	// Step 3: Enter scroll mode
	err = previewPane.ScrollUp(setup.instance)
	require.NoError(t, err)

	// Verify we entered scrolling mode
	require.True(t, previewPane.isScrolling, "Should be in scrolling mode after ScrollUp")

	// Step 4: Get the content directly from the viewport
	viewportContent := previewPane.viewport.View()
	t.Logf("Viewport content: %q", viewportContent)

	// With proper implementation, the viewport should have the full history content
	// Note: The viewport will be positioned at the bottom initially, so we need to scroll up

	// Step 5: Scroll up multiple times to get to the top
	for range 50 {
		err = previewPane.ScrollUp(setup.instance)
		require.NoError(t, err)
	}

	// Now get the viewport content after scrolling up
	viewportAfterScrollUp := previewPane.viewport.View()
	t.Logf("Viewport after scrolling up: %q", viewportAfterScrollUp)

	// Step 6: Scroll down multiple times
	for range 25 {
		err = previewPane.ScrollDown(setup.instance)
		require.NoError(t, err)
	}

	// Get updated viewport content after scrolling down
	viewportAfterScrollDown := previewPane.viewport.View()
	t.Logf("Viewport after scrolling down: %q", viewportAfterScrollDown)

	// Step 7: Reset to normal mode
	err = previewPane.ResetToNormalMode(setup.instance)
	require.NoError(t, err)

	// Verify we exited scrolling mode
	require.False(t, previewPane.isScrolling, "Should not be in scrolling mode after reset")
}

// TestPreviewContentWithoutScrolling tests that the preview pane correctly displays content
// for a new instance without requiring scrolling
func TestPreviewContentWithoutScrolling(t *testing.T) {
	// Define session name upfront so mock can return correct value
	sessionName := "test-preview-content"

	// Create test content
	expectedContent := "$ echo test\ntest"

	// Track session creation state
	sessionCreated := false

	// Mock command execution
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			cmdStr := cmd.String()

			// Handle zellij session existence checking
			if strings.Contains(cmdStr, "list-sessions") {
				if sessionCreated {
					return nil // Session exists
				} else {
					return fmt.Errorf("session does not exist")
				}
			}

			// Handle session creation
			if strings.Contains(cmdStr, "zellij") && strings.Contains(cmdStr, "attach") {
				sessionCreated = true
				return nil
			}

			// Handle dump-screen commands - write content to the temp file
			if strings.Contains(cmdStr, "dump-screen") {
				// Get the file path (last argument)
				args := cmd.Args
				if len(args) > 0 {
					tmpFile := args[len(args)-1]
					os.WriteFile(tmpFile, []byte(expectedContent), 0644)
				}
				return nil
			}

			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			cmdStr := cmd.String()

			// Handle list-sessions for session check - return the correct session name
			if strings.Contains(cmdStr, "list-sessions") {
				if sessionCreated {
					return []byte("claudesquad_" + sessionName), nil
				}
				return []byte(""), fmt.Errorf("no sessions")
			}

			return []byte(""), nil
		},
	}

	// Setup test environment
	setup := setupTestEnvironment(t, cmdExec, sessionName)
	defer setup.cleanupFn()

	// Create the preview pane
	previewPane := NewPreviewPane()
	previewPane.SetSize(80, 30) // Set reasonable size for testing

	// Update the preview content (this should display the content without scrolling)
	err := previewPane.UpdateContent(setup.instance)
	require.NoError(t, err)

	// Verify we're not in scrolling mode
	require.False(t, previewPane.isScrolling, "Should not be in scrolling mode")

	// Verify that the preview state is not in fallback mode
	require.False(t, previewPane.previewState.fallback, "Preview should not be in fallback mode")

	// Verify that the preview state contains the expected content
	require.Equal(t, expectedContent, previewPane.previewState.text, "Preview state should contain the expected content")

	// Verify the rendered string contains the content
	renderedString := previewPane.String()
	require.Contains(t, renderedString, "test", "Rendered preview should contain the test content")
}

// Helper function for max
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
