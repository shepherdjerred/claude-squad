package overlay

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FileEntry represents a file or directory in the file browser
type FileEntry struct {
	Name        string
	Path        string
	IsDir       bool
	IsGitDir    bool
	Expanded    bool
	Depth       int
	Parent      *FileEntry
	Children    []*FileEntry
	IsSpecial   bool   // For special entries like "." current directory
	DisplayName string // Optional display name override
}

// FileBrowserOverlay represents a file browser overlay for selecting directories
type FileBrowserOverlay struct {
	root          *FileEntry
	entries       []*FileEntry // Flattened list for display
	selectedIdx   int
	Submitted     bool
	Canceled      bool
	SelectedPath  string
	width, height int
	scrollOffset  int
	message       string    // Feedback message to display
	messageTime   time.Time // When the message was set
	cwdIsGitRepo  bool      // Whether current working directory is a git repo
	cwdPath       string    // The current working directory path
}

// NewFileBrowserOverlay creates a new file browser overlay starting at the given path
func NewFileBrowserOverlay(startPath string) (*FileBrowserOverlay, error) {
	// Expand ~ to home directory
	if strings.HasPrefix(startPath, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		startPath = filepath.Join(home, startPath[1:])
	}

	// Get absolute path
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return nil, err
	}

	fb := &FileBrowserOverlay{
		selectedIdx:  0,
		cwdPath:      absPath,
		cwdIsGitRepo: isGitRepo(absPath),
	}

	// Create root entry
	fb.root = &FileEntry{
		Name:     filepath.Base(absPath),
		Path:     absPath,
		IsDir:    true,
		IsGitDir: isGitRepo(absPath),
		Expanded: true,
		Depth:    0,
	}

	// Load children of root
	if err := fb.loadChildren(fb.root); err != nil {
		return nil, err
	}

	// Flatten entries for display
	fb.flattenEntries()

	return fb, nil
}

// isGitRepo checks if the given path is a git repository
func isGitRepo(path string) bool {
	gitPath := filepath.Join(path, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return false
	}
	// .git can be a directory (normal repo) or a file (worktree)
	return info.IsDir() || !info.IsDir()
}

// loadChildren loads the children of a directory entry
func (fb *FileBrowserOverlay) loadChildren(entry *FileEntry) error {
	if !entry.IsDir {
		return nil
	}

	dirEntries, err := os.ReadDir(entry.Path)
	if err != nil {
		return err
	}

	entry.Children = make([]*FileEntry, 0)

	for _, de := range dirEntries {
		name := de.Name()

		// Skip hidden files
		if strings.HasPrefix(name, ".") {
			continue
		}

		if !de.IsDir() {
			continue // Only show directories
		}

		childPath := filepath.Join(entry.Path, name)
		child := &FileEntry{
			Name:     name,
			Path:     childPath,
			IsDir:    true,
			IsGitDir: isGitRepo(childPath),
			Expanded: false,
			Depth:    entry.Depth + 1,
			Parent:   entry,
		}
		entry.Children = append(entry.Children, child)
	}

	// Sort children: git repos first, then alphabetically
	sort.Slice(entry.Children, func(i, j int) bool {
		// Git repos come first
		if entry.Children[i].IsGitDir != entry.Children[j].IsGitDir {
			return entry.Children[i].IsGitDir
		}
		return entry.Children[i].Name < entry.Children[j].Name
	})

	return nil
}

// flattenEntries creates a flat list of entries for display
func (fb *FileBrowserOverlay) flattenEntries() {
	fb.entries = make([]*FileEntry, 0)

	// Add special "current directory" entry if root is a git repo
	if fb.root.IsGitDir {
		currentDirEntry := &FileEntry{
			Name:        ".",
			DisplayName: ". (current directory)",
			Path:        fb.root.Path,
			IsDir:       true,
			IsGitDir:    true,
			IsSpecial:   true,
			Depth:       0,
		}
		fb.entries = append(fb.entries, currentDirEntry)
	}

	fb.flattenEntry(fb.root)
}

func (fb *FileBrowserOverlay) flattenEntry(entry *FileEntry) {
	fb.entries = append(fb.entries, entry)

	if entry.Expanded && entry.Children != nil {
		for _, child := range entry.Children {
			fb.flattenEntry(child)
		}
	}
}

// SetSize sets the size of the file browser
func (fb *FileBrowserOverlay) SetSize(width, height int) {
	fb.width = width
	fb.height = height
}

// setMessage sets a temporary feedback message
func (fb *FileBrowserOverlay) setMessage(msg string) {
	fb.message = msg
	fb.messageTime = time.Now()
}

// getMessage returns the current message if it's still valid (within 2 seconds)
func (fb *FileBrowserOverlay) getMessage() string {
	if fb.message != "" && time.Since(fb.messageTime) < 2*time.Second {
		return fb.message
	}
	fb.message = ""
	return ""
}

// HandleKeyPress processes a key press and updates the state accordingly
// Returns true if the overlay should be closed
func (fb *FileBrowserOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyUp:
		if fb.selectedIdx > 0 {
			fb.selectedIdx--
			fb.adjustScroll()
		}
		return false
	case tea.KeyDown:
		if fb.selectedIdx < len(fb.entries)-1 {
			fb.selectedIdx++
			fb.adjustScroll()
		}
		return false
	case tea.KeyRight:
		// Expand directory
		if fb.selectedIdx < len(fb.entries) {
			entry := fb.entries[fb.selectedIdx]
			if entry.IsDir && !entry.Expanded && !entry.IsSpecial {
				entry.Expanded = true
				if entry.Children == nil {
					fb.loadChildren(entry)
				}
				fb.flattenEntries()
			}
		}
		return false
	case tea.KeyLeft:
		// Collapse directory or go to parent
		if fb.selectedIdx < len(fb.entries) {
			entry := fb.entries[fb.selectedIdx]
			if entry.IsSpecial {
				return false
			}
			if entry.Expanded && entry.Children != nil && len(entry.Children) > 0 {
				entry.Expanded = false
				fb.flattenEntries()
			} else if entry.Parent != nil {
				// Find parent in entries and select it
				for i, e := range fb.entries {
					if e == entry.Parent {
						fb.selectedIdx = i
						fb.adjustScroll()
						break
					}
				}
			}
		}
		return false
	case tea.KeyEnter:
		// Select current directory if it's a git repo
		if fb.selectedIdx < len(fb.entries) {
			entry := fb.entries[fb.selectedIdx]
			if entry.IsGitDir {
				fb.SelectedPath = entry.Path
				fb.Submitted = true
				return true
			}
			// If not a git repo, show feedback and toggle expand
			fb.setMessage("Not a git repository - expand to find repos inside")
			if entry.IsDir && !entry.IsSpecial {
				entry.Expanded = !entry.Expanded
				if entry.Expanded && entry.Children == nil {
					fb.loadChildren(entry)
				}
				fb.flattenEntries()
			}
		}
		return false
	case tea.KeyEsc:
		fb.Canceled = true
		return true
	}

	// Handle vim-style navigation and other keys
	switch msg.String() {
	case "j":
		if fb.selectedIdx < len(fb.entries)-1 {
			fb.selectedIdx++
			fb.adjustScroll()
		}
	case "k":
		if fb.selectedIdx > 0 {
			fb.selectedIdx--
			fb.adjustScroll()
		}
	case "l":
		// Expand directory
		if fb.selectedIdx < len(fb.entries) {
			entry := fb.entries[fb.selectedIdx]
			if entry.IsDir && !entry.Expanded && !entry.IsSpecial {
				entry.Expanded = true
				if entry.Children == nil {
					fb.loadChildren(entry)
				}
				fb.flattenEntries()
			}
		}
	case "h":
		// Collapse directory or go to parent
		if fb.selectedIdx < len(fb.entries) {
			entry := fb.entries[fb.selectedIdx]
			if entry.IsSpecial {
				break
			}
			if entry.Expanded && entry.Children != nil && len(entry.Children) > 0 {
				entry.Expanded = false
				fb.flattenEntries()
			} else if entry.Parent != nil {
				for i, e := range fb.entries {
					if e == entry.Parent {
						fb.selectedIdx = i
						fb.adjustScroll()
						break
					}
				}
			}
		}
	case "~":
		// Go to home directory
		home, err := os.UserHomeDir()
		if err == nil {
			fb.NavigateToPath(home)
		}
	case "u", "-":
		// Go up to parent directory
		fb.GoUp()
	case "g":
		// Go to first entry
		fb.selectedIdx = 0
		fb.scrollOffset = 0
	case "G":
		// Go to last entry
		fb.selectedIdx = len(fb.entries) - 1
		fb.adjustScroll()
	}

	return false
}

// adjustScroll adjusts the scroll offset to keep the selected item visible
func (fb *FileBrowserOverlay) adjustScroll() {
	visibleRows := fb.getVisibleRows()
	if visibleRows <= 0 {
		return
	}

	if fb.selectedIdx < fb.scrollOffset {
		fb.scrollOffset = fb.selectedIdx
	} else if fb.selectedIdx >= fb.scrollOffset+visibleRows {
		fb.scrollOffset = fb.selectedIdx - visibleRows + 1
	}
}

// getVisibleRows returns the number of visible rows in the file browser
func (fb *FileBrowserOverlay) getVisibleRows() int {
	// Account for title, subtitle, path, border, padding, help text, message
	return fb.height - 12
}

// IsSubmitted returns whether the form was submitted
func (fb *FileBrowserOverlay) IsSubmitted() bool {
	return fb.Submitted
}

// IsCanceled returns whether the form was canceled
func (fb *FileBrowserOverlay) IsCanceled() bool {
	return fb.Canceled
}

// GetSelectedPath returns the selected path
func (fb *FileBrowserOverlay) GetSelectedPath() string {
	return fb.SelectedPath
}

// View renders the file browser
func (fb *FileBrowserOverlay) View() string {
	return fb.Render()
}

// Render renders the file browser overlay
func (fb *FileBrowserOverlay) Render() string {
	// Styles
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Bold(true)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Italic(true)

	pathStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666"))

	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("0")).
		Bold(true)

	gitRepoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#51bd73")).
		Bold(true)

	dirStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888"))

	specialStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#51bd73")).
		Bold(true).
		Italic(true)

	helpKeyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Bold(true)

	helpDescStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666"))

	messageStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#de613e")).
		Italic(true)

	separatorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#444444"))

	// Build the view
	content := titleStyle.Render("Select a Git Repository") + "\n"
	content += subtitleStyle.Render("Git repos shown in green - press Enter to select") + "\n"
	content += pathStyle.Render(fb.root.Path) + "\n"
	content += separatorStyle.Render(strings.Repeat("─", fb.width-6)) + "\n"

	visibleRows := fb.getVisibleRows()
	if visibleRows < 1 {
		visibleRows = 10
	}

	// Determine visible range
	startIdx := fb.scrollOffset
	endIdx := fb.scrollOffset + visibleRows
	if endIdx > len(fb.entries) {
		endIdx = len(fb.entries)
	}

	for i := startIdx; i < endIdx; i++ {
		entry := fb.entries[i]

		// Build indent (special entries have no indent)
		var indent string
		if entry.IsSpecial {
			indent = ""
		} else {
			indent = strings.Repeat("  ", entry.Depth)
		}

		// Build prefix (folder icon + expand indicator)
		var prefix string
		if entry.IsSpecial {
			prefix = ""
		} else if entry.IsDir {
			if entry.Expanded {
				prefix = "v "
			} else {
				prefix = "> "
			}
		}

		// Build the icon and name
		var displayName string
		if entry.DisplayName != "" {
			displayName = entry.DisplayName
		} else {
			displayName = entry.Name
		}

		var icon string
		if entry.IsGitDir {
			icon = "[git] "
		} else {
			icon = "[dir] "
		}

		line := indent + prefix + icon + displayName

		// Truncate if too long
		maxWidth := fb.width - 8
		if maxWidth > 0 && len(line) > maxWidth {
			line = line[:maxWidth-3] + "..."
		}

		// Apply styling
		if i == fb.selectedIdx {
			// Pad to full width for better selection visibility
			padWidth := fb.width - 8
			if len(line) < padWidth {
				line = line + strings.Repeat(" ", padWidth-len(line))
			}
			line = selectedStyle.Render(line)
		} else if entry.IsSpecial {
			line = specialStyle.Render(line)
		} else if entry.IsGitDir {
			line = gitRepoStyle.Render(line)
		} else {
			line = dirStyle.Render(line)
		}

		content += line + "\n"
	}

	// Add scroll indicator if needed
	if len(fb.entries) > visibleRows {
		scrollInfo := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Render(fmt.Sprintf("  (%d-%d of %d)", fb.scrollOffset+1, endIdx, len(fb.entries)))
		content += scrollInfo + "\n"
	} else {
		content += "\n"
	}

	// Add message if present
	if msg := fb.getMessage(); msg != "" {
		content += messageStyle.Render(msg) + "\n"
	} else {
		content += "\n"
	}

	content += separatorStyle.Render(strings.Repeat("─", fb.width-6)) + "\n"

	// Add help text in a more readable format
	helpLines := []struct{ key, desc string }{
		{"↑/k ↓/j", "navigate"},
		{"←/h →/l", "collapse/expand"},
		{"Enter", "select repo"},
		{"-/u", "parent dir"},
		{"~", "home"},
		{"Esc", "cancel"},
	}

	var helpParts []string
	for _, h := range helpLines {
		helpParts = append(helpParts, helpKeyStyle.Render(h.key)+helpDescStyle.Render(" "+h.desc))
	}
	content += strings.Join(helpParts, helpDescStyle.Render(" • "))

	return style.Render(content)
}

// NavigateToPath navigates to a specific path
func (fb *FileBrowserOverlay) NavigateToPath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	fb.root = &FileEntry{
		Name:     filepath.Base(absPath),
		Path:     absPath,
		IsDir:    true,
		IsGitDir: isGitRepo(absPath),
		Expanded: true,
		Depth:    0,
	}

	if err := fb.loadChildren(fb.root); err != nil {
		return err
	}

	fb.flattenEntries()
	fb.selectedIdx = 0
	fb.scrollOffset = 0

	return nil
}

// GoUp navigates to the parent directory
func (fb *FileBrowserOverlay) GoUp() error {
	parentPath := filepath.Dir(fb.root.Path)
	if parentPath == fb.root.Path {
		// Already at root
		return nil
	}

	return fb.NavigateToPath(parentPath)
}
