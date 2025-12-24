package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrClaudeProjectNotFound is returned when Claude's project directory doesn't exist yet.
// This is expected for new instances that haven't run Claude.
var ErrClaudeProjectNotFound = errors.New("claude project directory not found")

// ErrNoSessionFiles is returned when no session files are found in the Claude project directory.
var ErrNoSessionFiles = errors.New("no session files found")

// ExtractClaudeSessionID extracts the most recent Claude session ID from Claude's project files.
// Claude stores session data in ~/.claude/projects/{project-dir}/ where project-dir is a
// transformed version of the worktree path.
// Returns ErrClaudeProjectNotFound if the directory doesn't exist yet (expected for new instances).
func ExtractClaudeSessionID(worktreePath string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Convert worktree path to Claude project directory format
	// e.g., /Users/jerred/.claude-squad/worktrees/jerred/colors_18840af3cf6904f0
	// becomes: -Users-jerred--claude-squad-worktrees-jerred-colors-18840af3cf6904f0
	projectDirName := pathToClaudeProjectDir(worktreePath)
	projectDir := filepath.Join(homeDir, ".claude", "projects", projectDirName)

	// Check if the project directory exists
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return "", ErrClaudeProjectNotFound
	}

	// Find the most recent .jsonl file (excluding agent- files)
	files, err := os.ReadDir(projectDir)
	if err != nil {
		return "", fmt.Errorf("failed to read project directory: %w", err)
	}

	var sessionFiles []os.DirEntry
	for _, f := range files {
		name := f.Name()
		// Look for .jsonl files that are not agent files
		if strings.HasSuffix(name, ".jsonl") && !strings.HasPrefix(name, "agent-") {
			sessionFiles = append(sessionFiles, f)
		}
	}

	if len(sessionFiles) == 0 {
		return "", ErrNoSessionFiles
	}

	// Sort by modification time (newest first)
	sort.Slice(sessionFiles, func(i, j int) bool {
		iInfo, err1 := sessionFiles[i].Info()
		jInfo, err2 := sessionFiles[j].Info()
		if err1 != nil || err2 != nil {
			return false
		}
		return iInfo.ModTime().After(jInfo.ModTime())
	})

	// Read the most recent file and extract sessionId
	sessionFilePath := filepath.Join(projectDir, sessionFiles[0].Name())
	return extractSessionIDFromJSONL(sessionFilePath)
}

// pathToClaudeProjectDir converts a filesystem path to Claude's project directory format.
// Claude replaces / with - in the path.
func pathToClaudeProjectDir(path string) string {
	// Replace all slashes with dashes
	result := strings.ReplaceAll(path, "/", "-")

	// Claude keeps a leading dash for absolute paths
	if !strings.HasPrefix(result, "-") && strings.HasPrefix(path, "/") {
		result = "-" + result
	}

	return result
}

// sessionMessage represents a line in Claude's session .jsonl file
type sessionMessage struct {
	SessionID string `json:"sessionId"`
}

// extractSessionIDFromJSONL reads a .jsonl file and extracts the sessionId
func extractSessionIDFromJSONL(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open session file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Set a larger buffer for potentially long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		var msg sessionMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			// Skip lines that don't parse as JSON
			continue
		}
		if msg.SessionID != "" {
			return msg.SessionID, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading session file: %w", err)
	}

	return "", fmt.Errorf("no session ID found in file: %s", filePath)
}
