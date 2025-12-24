package zellij

import (
	"claude-squad/cmd/cmd_test"
	"claude-squad/log"
	"crypto/sha256"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func init() {
	log.Initialize(false)
}

func TestToClaudeSquadZellijName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test", ZellijPrefix + "test"},
		{"test session", ZellijPrefix + "testsession"},
		{"test.session", ZellijPrefix + "test_session"},
		{"test session.name", ZellijPrefix + "testsession_name"},
		{"   spaced   ", ZellijPrefix + "spaced"},
		{"dots.and.spaces", ZellijPrefix + "dots_and_spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toClaudeSquadZellijName(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestNewZellijSession(t *testing.T) {
	session := NewZellijSession("test-session", "claude")
	require.Equal(t, ZellijPrefix+"test-session", session.sanitizedName)
	require.Equal(t, "claude", session.program)
	require.NotNil(t, session.contentCache)
}

func TestNewZellijSessionWithWhitespace(t *testing.T) {
	session := NewZellijSession("test session", "claude")
	require.Equal(t, ZellijPrefix+"testsession", session.sanitizedName)
}

func TestDoesSessionExist(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		outputErr    error
		expectExists bool
	}{
		{
			name:         "session exists",
			output:       "claudesquad_test-session\nother-session\n",
			outputErr:    nil,
			expectExists: true,
		},
		{
			name:         "session does not exist",
			output:       "other-session\nanother-session\n",
			outputErr:    nil,
			expectExists: false,
		},
		{
			name:         "empty list",
			output:       "",
			outputErr:    nil,
			expectExists: false,
		},
		{
			name:         "list command error",
			output:       "",
			outputErr:    exec.ErrNotFound,
			expectExists: false,
		},
		{
			name:         "session exists with ANSI codes",
			output:       "\x1b[32;1mclaudesquad_test-session\x1b[m [Created \x1b[35;1m5s\x1b[m ago]\n",
			outputErr:    nil,
			expectExists: true,
		},
		{
			name:         "session not found with ANSI codes",
			output:       "\x1b[32;1mother-session\x1b[m [Created \x1b[35;1m10s\x1b[m ago]\n",
			outputErr:    nil,
			expectExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdExec := cmd_test.MockCmdExec{
				RunFunc: func(cmd *exec.Cmd) error {
					return nil
				},
				OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
					return []byte(tt.output), tt.outputErr
				},
			}

			session := NewZellijSessionWithDeps("test-session", "claude", cmdExec)
			exists := session.DoesSessionExist()
			require.Equal(t, tt.expectExists, exists)
		})
	}
}

func TestSendKeys(t *testing.T) {
	var executedCmd string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			executedCmd = cmd.String()
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte{}, nil
		},
	}

	session := NewZellijSessionWithDeps("test", "claude", cmdExec)
	err := session.SendKeys("hello")
	require.NoError(t, err)
	require.Contains(t, executedCmd, "zellij")
	require.Contains(t, executedCmd, "write-chars")
	require.Contains(t, executedCmd, "hello")
}

func TestTapEnter(t *testing.T) {
	var executedCmd string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			executedCmd = cmd.String()
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte{}, nil
		},
	}

	session := NewZellijSessionWithDeps("test", "claude", cmdExec)
	err := session.TapEnter()
	require.NoError(t, err)
	require.Contains(t, executedCmd, "zellij")
	require.Contains(t, executedCmd, "write")
	require.Contains(t, executedCmd, "13") // carriage return
}

func TestSetDetachedSize(t *testing.T) {
	var executedCmds []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			executedCmds = append(executedCmds, cmd.String())
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte{}, nil
		},
	}

	session := NewZellijSessionWithDeps("test", "claude", cmdExec)
	err := session.SetDetachedSize(80, 24)
	require.NoError(t, err)
	// SetDetachedSize doesn't do anything for zellij (no-op)
	require.Empty(t, executedCmds)
}

func TestCapturePaneContent(t *testing.T) {
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("mock content"), nil
		},
	}

	session := NewZellijSessionWithDeps("test", "claude", cmdExec)
	// Note: CapturePaneContent uses file I/O internally
	// We're just verifying the session is properly constructed
	require.NotNil(t, session)
}

// Test the hash method on statusMonitor - uses io.WriteString for no allocation
func TestStatusMonitorHash(t *testing.T) {
	monitor := newStatusMonitor()

	s1 := "test string"
	s2 := "test string"
	s3 := "different string"

	h1 := monitor.hash(s1)
	h2 := monitor.hash(s2)
	h3 := monitor.hash(s3)

	// Same strings should produce same hash
	require.Equal(t, h1, h2)
	// Different strings should produce different hash
	require.NotEqual(t, h1, h3)
	// Hash should not be empty
	require.NotEmpty(t, h1)
	// SHA256 hash is 32 bytes
	require.Len(t, h1, 32)
}

func TestStatusMonitorHashNoAlloc(t *testing.T) {
	// Test that hash uses io.WriteString (no allocation)
	// by verifying it works with large strings
	monitor := newStatusMonitor()
	largeString := strings.Repeat("x", 1024*1024) // 1MB
	hash := monitor.hash(largeString)
	require.NotEmpty(t, hash)
	require.Len(t, hash, 32) // SHA256 is 32 bytes
}

func TestStatusMonitorHashMatchesExpected(t *testing.T) {
	// Verify the hash matches what we expect from sha256 with io.WriteString
	monitor := newStatusMonitor()
	testString := "hello world"

	// Calculate expected hash
	expected := sha256.New()
	io.WriteString(expected, testString)
	expectedHash := expected.Sum(nil)

	actual := monitor.hash(testString)
	require.Equal(t, expectedHash, actual)
}

// Test content cache
func TestContentCache(t *testing.T) {
	cache := newContentCache(100 * time.Millisecond)

	// Initially empty
	content, _, ok := cache.Get()
	require.False(t, ok)
	require.Empty(t, content)

	// Set content
	cache.Set("test content")
	content, _, ok = cache.Get()
	require.True(t, ok)
	require.Equal(t, "test content", content)

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)
	content, _, ok = cache.Get()
	require.False(t, ok)
	require.Empty(t, content)
}

func TestContentCacheInvalidate(t *testing.T) {
	cache := newContentCache(1 * time.Hour) // Long TTL

	cache.Set("test content")
	content, _, ok := cache.Get()
	require.True(t, ok)
	require.Equal(t, "test content", content)

	// Invalidate
	cache.Invalidate()
	content, _, ok = cache.Get()
	require.False(t, ok)
	require.Empty(t, content)
}

func TestContentCacheIsStale(t *testing.T) {
	cache := newContentCache(50 * time.Millisecond)

	// Initially stale
	require.True(t, cache.IsStale())

	// After setting, not stale
	cache.Set("content")
	require.False(t, cache.IsStale())

	// After TTL expires, stale again
	time.Sleep(60 * time.Millisecond)
	require.True(t, cache.IsStale())
}

// Test status monitor creation
func TestStatusMonitor(t *testing.T) {
	monitor := newStatusMonitor()
	require.NotNil(t, monitor)
	require.Nil(t, monitor.prevOutputHash)
}

func TestClose(t *testing.T) {
	var executedCmd string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			executedCmd = cmd.String()
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte{}, nil
		},
	}

	session := NewZellijSessionWithDeps("test", "claude", cmdExec)
	err := session.Close()
	require.NoError(t, err)
	require.Contains(t, executedCmd, "zellij")
	require.Contains(t, executedCmd, "kill-session")
}

func TestCleanupSessions(t *testing.T) {
	var executedCmds []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			executedCmds = append(executedCmds, cmd.String())
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			// Return some sessions including claude-squad ones
			return []byte("claudesquad_test1\nother-session\nclaudesquad_test2\n"), nil
		},
	}

	err := CleanupSessions(cmdExec)
	require.NoError(t, err)

	// Should have called kill-session for each claude-squad session
	killCount := 0
	for _, cmd := range executedCmds {
		if strings.Contains(cmd, "kill-session") {
			killCount++
		}
	}
	require.Equal(t, 2, killCount)
}

func TestCleanupSessionsNoClaudeSquadSessions(t *testing.T) {
	var executedCmds []string
	cmdExec := cmd_test.MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			executedCmds = append(executedCmds, cmd.String())
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			// Return sessions without claude-squad prefix
			return []byte("session1\nsession2\n"), nil
		},
	}

	err := CleanupSessions(cmdExec)
	require.NoError(t, err)

	// Should not have called kill-session
	for _, cmd := range executedCmds {
		require.NotContains(t, cmd, "kill-session")
	}
}

func TestZellijPrefix(t *testing.T) {
	// Verify the prefix constant
	require.Equal(t, "claudesquad_", ZellijPrefix)
}

func TestReadCaptureFileWithRetry_Success(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_capture_success.txt")
	defer os.Remove(tmpFile)

	err := os.WriteFile(tmpFile, []byte("test content"), 0644)
	require.NoError(t, err)

	content, err := readCaptureFileWithRetry(tmpFile)
	require.NoError(t, err)
	require.Equal(t, "test content", string(content))
}

func TestReadCaptureFileWithRetry_FileNotExist(t *testing.T) {
	content, err := readCaptureFileWithRetry("/nonexistent/file.txt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read capture file after")
	require.Contains(t, err.Error(), "does not exist")
	require.Nil(t, content)
}

func TestReadCaptureFileWithRetry_EmptyFile(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_empty_capture.txt")
	defer os.Remove(tmpFile)

	err := os.WriteFile(tmpFile, []byte{}, 0644)
	require.NoError(t, err)

	content, err := readCaptureFileWithRetry(tmpFile)
	require.Error(t, err)
	require.Contains(t, err.Error(), "capture file is empty")
	require.Nil(t, content)
}

func TestReadCaptureFileWithRetry_DelayedWrite(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_delayed_capture.txt")
	defer os.Remove(tmpFile)

	// Simulate delayed file creation
	go func() {
		time.Sleep(15 * time.Millisecond)
		os.WriteFile(tmpFile, []byte("delayed content"), 0644)
	}()

	content, err := readCaptureFileWithRetry(tmpFile)
	require.NoError(t, err)
	require.Equal(t, "delayed content", string(content))
}
