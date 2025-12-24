package ui

import (
	"errors"
	"fmt"
	"strings"

	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/ui/layout"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// Status icons with semantic meaning (used with colors for accessibility)
const (
	readyIcon   = "● "  // Ready state
	pausedIcon  = "⏸ " // Paused state
	runningIcon = "◐ "  // Running state (when no spinner available)
)

// compactModeThreshold is the height below which the list switches to compact mode
const compactModeThreshold = 35

// titleAreaHeight is the number of lines used by the title area (2 newlines + title + 2 newlines)
const titleAreaHeight = 5

// Status styles using semantic colors from styles.go
var readyStyle = lipgloss.NewStyle().
	Foreground(StatusSuccess)

var runningStyle = lipgloss.NewStyle().
	Foreground(StatusRunning)

var addedLinesStyle = lipgloss.NewStyle().
	Foreground(StatusSuccess)

var removedLinesStyle = lipgloss.NewStyle().
	Foreground(StatusError)

var pausedStyle = lipgloss.NewStyle().
	Foreground(StatusPaused)

var titleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"})

var listDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Foreground(lipgloss.AdaptiveColor{Light: "#A49FA5", Dark: "#777777"})

var selectedTitleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Background(lipgloss.Color("#dde4f0")).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#1a1a1a"})

var selectedDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Background(lipgloss.Color("#dde4f0")).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#1a1a1a"})

var mainTitle = lipgloss.NewStyle().
	Background(lipgloss.Color("62")).
	Foreground(lipgloss.Color("230"))

var autoYesStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("#dde4f0")).
	Foreground(lipgloss.Color("#1a1a1a"))

var muxTagStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"}).
	Italic(true)

var timerStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"}).
	Italic(true)

var summaryStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"}).
	Italic(true)

var selectedSummaryStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#444444", Dark: "#444444"}).
	Italic(true)

var filterStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#888888"})

var filterActiveStyle = lipgloss.NewStyle().
	Foreground(Primary).
	Bold(true).
	Underline(true)

var filterInactiveStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"})

// Compact mode styles with minimal padding
var compactTitleStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"})

var compactSelectedStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Background(lipgloss.Color("#dde4f0")).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#1a1a1a"})

var scrollIndicatorStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"})

// FilterMode represents the current filter for the list view
type FilterMode int

const (
	FilterAll FilterMode = iota
	FilterNeedsAttention
	FilterArchived
)

type List struct {
	items         []*session.Instance
	selectedIdx   int
	height, width int
	renderer      *InstanceRenderer
	autoyes       bool

	// map of repo name to number of instances using it. Used to display the repo name only if there are
	// multiple repos in play.
	repos map[string]int

	// filterMode controls the current filter view (ALL, NEEDS ATTENTION, or ARCHIVED)
	filterMode FilterMode

	// scrollOffset is the index of the first visible item in the list
	scrollOffset int

	// compactMode is true when the list is in compact mode (smaller terminal)
	compactMode bool

	// degradation holds the current UI degradation flags
	degradation layout.Degradation
}

func NewList(spinner *spinner.Model, autoYes bool) *List {
	return &List{
		items:    []*session.Instance{},
		renderer: &InstanceRenderer{spinner: spinner},
		repos:    make(map[string]int),
		autoyes:  autoYes,
	}
}

// SetSize sets the height and width of the list.
func (l *List) SetSize(width, height int) {
	l.width = width
	l.height = height
	l.renderer.setWidth(width)

	// Auto-detect compact mode based on height (can be overridden by SetDegradation)
	l.compactMode = height < compactModeThreshold
	l.renderer.compactMode = l.compactMode

	// Re-adjust scroll when size changes
	l.adjustScroll()
}

// SetDegradation applies layout degradation flags to the list.
func (l *List) SetDegradation(d layout.Degradation) {
	l.degradation = d
	// Override compact mode based on degradation (more restrictive wins)
	if d.IsCompactMode() && !l.compactMode {
		l.compactMode = true
		l.renderer.compactMode = true
	}
	l.renderer.degradation = d
}

// GetSelectedIndex returns the current selected index.
func (l *List) GetSelectedIndex() int {
	return l.selectedIdx
}

// getItemHeight returns the height in lines for a single item
func (l *List) getItemHeight(item *session.Instance) int {
	if l.compactMode {
		return 1 // Single line in compact mode
	}

	// Check degradation flags
	showDescription := l.degradation.ShouldShowDescription()
	showSummary := l.degradation.ShouldShowSummary()

	if !showDescription {
		// Minimal mode: title only (with padding)
		return 2
	}

	// Standard mode: title + branch row with padding
	baseHeight := 4
	if item.Summary != "" && showSummary {
		baseHeight = 5 // add summary line
	}
	return baseHeight
}

// getVisibleRows returns the number of rows available for list items
func (l *List) getVisibleRows() int {
	return l.height - l.degradation.GetTitleAreaHeight()
}

// calculateVisibleRange returns the start and end indices of items that fit in the visible area
func (l *List) calculateVisibleRange() (start, end int) {
	visibleItems := l.GetVisibleInstances()
	if len(visibleItems) == 0 {
		return 0, 0
	}

	availableHeight := l.getVisibleRows()
	if availableHeight <= 0 {
		return 0, 0
	}

	currentHeight := 0
	start = l.scrollOffset
	end = start

	for i := start; i < len(visibleItems); i++ {
		itemHeight := l.getItemHeight(visibleItems[i])
		if currentHeight+itemHeight <= availableHeight {
			end = i + 1
			currentHeight += itemHeight
		} else {
			break
		}
	}
	return start, end
}

// adjustScroll ensures the selected item is visible by adjusting scrollOffset
func (l *List) adjustScroll() {
	visibleItems := l.GetVisibleInstances()
	if len(visibleItems) == 0 {
		l.scrollOffset = 0
		return
	}

	// Ensure scrollOffset is valid
	if l.scrollOffset >= len(visibleItems) {
		l.scrollOffset = max(0, len(visibleItems)-1)
	}

	// If selected is above visible area, scroll up
	if l.selectedIdx < l.scrollOffset {
		l.scrollOffset = l.selectedIdx
		return
	}

	// Calculate how many items fit from current scroll offset
	availableHeight := l.getVisibleRows()
	if availableHeight <= 0 {
		return
	}

	currentHeight := 0
	visibleCount := 0

	for i := l.scrollOffset; i < len(visibleItems); i++ {
		itemHeight := l.getItemHeight(visibleItems[i])
		if currentHeight+itemHeight <= availableHeight {
			visibleCount++
			currentHeight += itemHeight
		} else {
			break
		}
	}

	// If selected is below visible area, scroll down
	if l.selectedIdx >= l.scrollOffset+visibleCount {
		// Find new offset that makes selected item visible at the bottom
		l.scrollOffset = l.selectedIdx - visibleCount + 1
		if l.scrollOffset < 0 {
			l.scrollOffset = 0
		}
	}
}

// SetSessionPreviewSize sets the height and width for the multiplexer sessions. This makes the stdout line have the correct
// width and height.
func (l *List) SetSessionPreviewSize(width, height int) (err error) {
	for i, item := range l.items {
		if !item.Started() || item.Paused() {
			continue
		}

		if innerErr := item.SetPreviewSize(width, height); innerErr != nil {
			err = errors.Join(
				err, fmt.Errorf("could not set preview size for instance %d: %v", i, innerErr))
		}
	}
	return
}

func (l *List) NumInstances() int {
	return len(l.GetVisibleInstances())
}

// NumAllInstances returns the total count of all instances (both archived and active)
func (l *List) NumAllInstances() int {
	return len(l.items)
}

// InstanceRenderer handles rendering of session.Instance objects
type InstanceRenderer struct {
	spinner     *spinner.Model
	width       int
	compactMode bool
	degradation layout.Degradation
}

func (r *InstanceRenderer) setWidth(width int) {
	r.width = AdjustPreviewWidth(width)
}

// ɹ and ɻ are other options.
const branchIcon = "Ꮧ"

// RenderCompact renders a single-line compact version of an instance for small screens
func (r *InstanceRenderer) RenderCompact(i *session.Instance, idx int, selected bool) string {
	prefix := fmt.Sprintf("%d.", idx)
	if idx >= 10 {
		prefix = fmt.Sprintf("%d.", idx)
	}

	// Status indicator
	var statusIcon string
	switch i.Status {
	case session.Running:
		statusIcon = r.spinner.View()
	case session.Ready:
		statusIcon = readyStyle.Render("●")
	case session.Paused:
		statusIcon = pausedStyle.Render("⏸")
	default:
		statusIcon = " "
	}

	// Branch (short, truncated)
	branch := i.Branch
	maxBranchLen := 15
	if len(branch) > maxBranchLen {
		branch = branch[:maxBranchLen-3] + "..."
	}

	// Calculate available width for title
	// Layout: prefix + space + statusIcon + space + title + space + [branch]
	fixedWidth := len(prefix) + 1 + 2 + 1 + 1 + len(branch) + 2 + 4 // extra padding
	maxTitleWidth := r.width - fixedWidth
	if maxTitleWidth < 10 {
		maxTitleWidth = 10
	}

	// Title (truncated)
	title := i.Title
	if len(title) > maxTitleWidth {
		if maxTitleWidth > 3 {
			title = title[:maxTitleWidth-3] + "..."
		} else {
			title = title[:maxTitleWidth]
		}
	}

	line := fmt.Sprintf("%s %s %s [%s]", prefix, statusIcon, title, branch)

	if selected {
		return compactSelectedStyle.Render(line)
	}
	return compactTitleStyle.Render(line)
}

func (r *InstanceRenderer) Render(i *session.Instance, idx int, selected bool, hasMultipleRepos bool) string {
	// Use compact rendering in compact mode
	if r.compactMode {
		return r.RenderCompact(i, idx, selected)
	}
	prefix := fmt.Sprintf(" %d. ", idx)
	if idx >= 10 {
		prefix = prefix[:len(prefix)-1]
	}
	titleS := selectedTitleStyle
	descS := selectedDescStyle
	if !selected {
		titleS = titleStyle
		descS = listDescStyle
	}

	// add spinner next to title if it's running
	var join string
	switch i.Status {
	case session.Running:
		join = fmt.Sprintf("%s ", r.spinner.View())
	case session.Ready:
		join = readyStyle.Render(readyIcon)
	case session.Paused:
		join = pausedStyle.Render(pausedIcon)
	default:
	}

	// Get multiplexer type tag
	muxTag := ""
	if mtype := i.GetMultiplexerType(); mtype != "" {
		muxTag = fmt.Sprintf(" [%s]", mtype)
	}

	// Build timer info (age and last opened) - only if not degraded
	var timerInfo string
	var timerInfoLen int
	if r.degradation.ShouldShowTimer() {
		ageStr := FormatRelativeTime(i.CreatedAt)
		openedStr := FormatLastOpened(i.LastOpenedAt)
		timerInfo = fmt.Sprintf("%s | opened %s", ageStr, openedStr)
		timerInfoLen = len(timerInfo)
	}

	// Cut the title if it's too long (account for mux tag and timer info)
	// Layout: [prefix][space][title][muxTag][spaces][timerInfo][space][icon]
	minSpacing := 2
	iconWidth := 3 // status icon width
	titleText := i.Title
	widthAvail := r.width - len(prefix) - 1 - len(muxTag) - minSpacing - timerInfoLen - iconWidth
	if widthAvail > 0 && widthAvail < len(titleText) {
		if widthAvail > 3 {
			titleText = titleText[:widthAvail-3] + "..."
		} else if widthAvail > 0 {
			titleText = titleText[:widthAvail]
		}
	}

	// Build title with multiplexer tag
	titleWithMux := titleText + muxTagStyle.Render(muxTag)

	// Calculate spacing to right-align timer info before the status icon
	leftContentLen := len(prefix) + 1 + len(titleText) + len(muxTag)
	rightContentLen := timerInfoLen + 1 + iconWidth
	spacesNeeded := r.width - leftContentLen - rightContentLen
	if spacesNeeded < minSpacing {
		spacesNeeded = minSpacing
	}
	spacing := strings.Repeat(" ", spacesNeeded)

	// Build the title line with timer info
	titleContent := fmt.Sprintf("%s %s%s%s", prefix, titleWithMux, spacing, timerStyle.Render(timerInfo))

	title := titleS.Render(lipgloss.JoinHorizontal(
		lipgloss.Left,
		titleContent,
		" ",
		join,
	))

	stat := i.GetDiffStats()

	var diff string
	var addedDiff, removedDiff string
	if stat == nil || stat.Error != nil || stat.IsEmpty() {
		// Don't show diff stats if there's an error or if they don't exist
		addedDiff = ""
		removedDiff = ""
		diff = ""
	} else {
		addedDiff = fmt.Sprintf("+%d", stat.Added)
		removedDiff = fmt.Sprintf("-%d ", stat.Removed)
		diff = lipgloss.JoinHorizontal(
			lipgloss.Center,
			addedLinesStyle.Background(descS.GetBackground()).Render(addedDiff),
			lipgloss.Style{}.Background(descS.GetBackground()).Foreground(descS.GetForeground()).Render(","),
			removedLinesStyle.Background(descS.GetBackground()).Render(removedDiff),
		)
	}

	remainingWidth := r.width
	remainingWidth -= len(prefix)
	remainingWidth -= len(branchIcon)

	diffWidth := len(addedDiff) + len(removedDiff)
	if diffWidth > 0 {
		diffWidth += 1
	}

	// Use fixed width for diff stats to avoid layout issues
	remainingWidth -= diffWidth

	branch := i.Branch
	if i.Started() && hasMultipleRepos {
		repoName, err := i.RepoName()
		if err != nil {
			log.ErrorLog.Printf("could not get repo name in instance renderer: %v", err)
		} else {
			branch += fmt.Sprintf(" (%s)", repoName)
		}
	}
	// Don't show branch if there's no space for it. Or show ellipsis if it's too long.
	if remainingWidth < 0 {
		branch = ""
	} else if remainingWidth < len(branch) {
		if remainingWidth < 3 {
			branch = ""
		} else {
			// We know the remainingWidth is at least 4 and branch is longer than that, so this is safe.
			branch = branch[:remainingWidth-3] + "..."
		}
	}
	remainingWidth -= len(branch)

	// Add spaces to fill the remaining width.
	spaces := ""
	if remainingWidth > 0 {
		spaces = strings.Repeat(" ", remainingWidth)
	}

	branchLine := fmt.Sprintf("%s %s-%s%s%s", strings.Repeat(" ", len(prefix)), branchIcon, branch, spaces, diff)

	// Build summary line if available and not degraded
	var summaryLine string
	if i.Summary != "" && r.degradation.ShouldShowSummary() {
		summaryText := i.Summary
		// Truncate summary if too long
		maxSummaryWidth := r.width - len(prefix) - 2
		if maxSummaryWidth > 0 && len(summaryText) > maxSummaryWidth {
			if maxSummaryWidth > 3 {
				summaryText = summaryText[:maxSummaryWidth-3] + "..."
			} else {
				summaryText = ""
			}
		}
		if summaryText != "" {
			sumStyle := summaryStyle
			if selected {
				sumStyle = selectedSummaryStyle.Background(descS.GetBackground())
			}
			summaryLine = fmt.Sprintf("%s %s", strings.Repeat(" ", len(prefix)), sumStyle.Render(summaryText))
		}
	}

	// join title, subtitle, and summary based on degradation flags
	var text string
	showDescription := r.degradation.ShouldShowDescription()

	if summaryLine != "" && showDescription {
		// Full view: title + branch + summary
		text = lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			descS.Render(branchLine),
			descS.Render(summaryLine),
		)
	} else if showDescription {
		// Standard view: title + branch
		text = lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			descS.Render(branchLine),
		)
	} else {
		// Minimal view: title only
		text = title
	}

	return text
}

func (l *List) String() string {
	titleText := " Instances "
	const autoYesText = " auto-yes "

	// Write the title.
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("\n")

	// Write title line
	// add padding of 2 because the border on list items adds some extra characters
	titleWidth := AdjustPreviewWidth(l.width) + 2
	if !l.autoyes {
		b.WriteString(lipgloss.Place(
			titleWidth, 1, lipgloss.Left, lipgloss.Bottom, mainTitle.Render(titleText)))
	} else {
		title := lipgloss.Place(
			titleWidth/2, 1, lipgloss.Left, lipgloss.Bottom, mainTitle.Render(titleText))
		autoYes := lipgloss.Place(
			titleWidth-(titleWidth/2), 1, lipgloss.Right, lipgloss.Bottom, autoYesStyle.Render(autoYesText))
		b.WriteString(lipgloss.JoinHorizontal(
			lipgloss.Top, title, autoYes))
	}

	b.WriteString("\n")

	// Write filter tabs with counts
	b.WriteString(l.renderFilterTabs())
	b.WriteString("\n")

	// Get visible instances based on archive view mode
	visibleItems := l.GetVisibleInstances()

	// Calculate visible range for scrolling
	start, end := l.calculateVisibleRange()

	// Render only the visible items
	for i := start; i < end && i < len(visibleItems); i++ {
		item := visibleItems[i]
		b.WriteString(l.renderer.Render(item, i+1, i == l.selectedIdx, len(l.repos) > 1))
		if i != end-1 && i != len(visibleItems)-1 {
			if l.compactMode {
				b.WriteString("\n")
			} else {
				b.WriteString("\n\n")
			}
		}
	}

	// Add scroll indicator if content is clipped and not degraded
	if len(visibleItems) > 0 && (start > 0 || end < len(visibleItems)) && !l.degradation.HideScrollIndicators {
		b.WriteString("\n")
		scrollInfo := fmt.Sprintf(" [%d-%d of %d]", start+1, end, len(visibleItems))
		b.WriteString(scrollIndicatorStyle.Render(scrollInfo))
	}

	return lipgloss.Place(l.width, l.height, lipgloss.Left, lipgloss.Top, b.String())
}

// Down selects the next item in the list. Wraps to the first item if at the end.
func (l *List) Down() {
	visibleItems := l.GetVisibleInstances()
	if len(visibleItems) == 0 {
		return
	}
	if l.selectedIdx < len(visibleItems)-1 {
		l.selectedIdx++
	} else {
		l.selectedIdx = 0
		l.scrollOffset = 0 // Reset scroll when wrapping to top
	}
	l.adjustScroll()
}

// Kill removes the selected instance from the list and kills it asynchronously.
func (l *List) Kill() {
	visibleItems := l.GetVisibleInstances()
	if len(visibleItems) == 0 {
		return
	}
	targetInstance := visibleItems[l.selectedIdx]

	// Find the actual index in the full items list
	actualIdx := -1
	for i, item := range l.items {
		if item == targetInstance {
			actualIdx = i
			break
		}
	}

	if actualIdx == -1 {
		log.ErrorLog.Printf("could not find instance in items list")
		return
	}

	// Unregister the reponame first (before removing from list).
	repoName, err := targetInstance.RepoName()
	if err != nil {
		log.ErrorLog.Printf("could not get repo name: %v", err)
	} else {
		l.rmRepo(repoName)
	}

	// If you delete the last one in the visible list, select the previous one.
	if l.selectedIdx == len(visibleItems)-1 {
		defer l.Up()
	}

	// Remove from the actual items list immediately.
	l.items = append(l.items[:actualIdx], l.items[actualIdx+1:]...)

	// Kill the zellij session and git worktree asynchronously to avoid blocking the UI.
	go func() {
		if err := targetInstance.Kill(); err != nil {
			log.ErrorLog.Printf("could not kill instance: %v", err)
		}
	}()
}

func (l *List) Attach() (chan struct{}, error) {
	visibleItems := l.GetVisibleInstances()
	if len(visibleItems) == 0 || l.selectedIdx >= len(visibleItems) {
		return nil, fmt.Errorf("no instance selected")
	}
	targetInstance := visibleItems[l.selectedIdx]
	return targetInstance.Attach()
}

// Up selects the prev item in the list. Wraps to the last item if at the beginning.
func (l *List) Up() {
	visibleItems := l.GetVisibleInstances()
	if len(visibleItems) == 0 {
		return
	}
	if l.selectedIdx > 0 {
		l.selectedIdx--
	} else {
		l.selectedIdx = len(visibleItems) - 1
		// Don't reset scroll - adjustScroll will handle positioning
	}
	l.adjustScroll()
}

// MoveUp moves the selected instance up in the list (swaps with previous).
// Returns true if the instance was moved, false otherwise.
func (l *List) MoveUp() bool {
	visibleItems := l.GetVisibleInstances()
	if len(visibleItems) <= 1 || l.selectedIdx <= 0 {
		return false
	}

	// Get the actual indices in the full items list
	currentItem := visibleItems[l.selectedIdx]
	prevItem := visibleItems[l.selectedIdx-1]

	currentIdx := -1
	prevIdx := -1
	for i, item := range l.items {
		if item == currentItem {
			currentIdx = i
		}
		if item == prevItem {
			prevIdx = i
		}
	}

	if currentIdx == -1 || prevIdx == -1 {
		return false
	}

	// Swap in the full list
	l.items[currentIdx], l.items[prevIdx] = l.items[prevIdx], l.items[currentIdx]
	l.selectedIdx--
	l.adjustScroll()
	return true
}

// MoveDown moves the selected instance down in the list (swaps with next).
// Returns true if the instance was moved, false otherwise.
func (l *List) MoveDown() bool {
	visibleItems := l.GetVisibleInstances()
	if len(visibleItems) <= 1 || l.selectedIdx >= len(visibleItems)-1 {
		return false
	}

	// Get the actual indices in the full items list
	currentItem := visibleItems[l.selectedIdx]
	nextItem := visibleItems[l.selectedIdx+1]

	currentIdx := -1
	nextIdx := -1
	for i, item := range l.items {
		if item == currentItem {
			currentIdx = i
		}
		if item == nextItem {
			nextIdx = i
		}
	}

	if currentIdx == -1 || nextIdx == -1 {
		return false
	}

	// Swap in the full list
	l.items[currentIdx], l.items[nextIdx] = l.items[nextIdx], l.items[currentIdx]
	l.selectedIdx++
	l.adjustScroll()
	return true
}

func (l *List) addRepo(repo string) {
	if _, ok := l.repos[repo]; !ok {
		l.repos[repo] = 0
	}
	l.repos[repo]++
}

func (l *List) rmRepo(repo string) {
	if _, ok := l.repos[repo]; !ok {
		log.ErrorLog.Printf("repo %s not found", repo)
		return
	}
	l.repos[repo]--
	if l.repos[repo] == 0 {
		delete(l.repos, repo)
	}
}

// AddInstance adds a new instance to the list. It returns a finalizer function that should be called when the instance
// is started. If the instance was restored from storage or is paused, you can call the finalizer immediately.
// When creating a new one and entering the name, you want to call the finalizer once the name is done.
func (l *List) AddInstance(instance *session.Instance) (finalize func()) {
	l.items = append(l.items, instance)
	// The finalizer registers the repo name once the instance is started.
	return func() {
		repoName, err := instance.RepoName()
		if err != nil {
			log.ErrorLog.Printf("could not get repo name: %v", err)
			return
		}

		l.addRepo(repoName)
	}
}

// GetSelectedInstance returns the currently selected instance from visible instances
func (l *List) GetSelectedInstance() *session.Instance {
	visibleItems := l.GetVisibleInstances()
	if len(visibleItems) == 0 || l.selectedIdx >= len(visibleItems) {
		return nil
	}
	return visibleItems[l.selectedIdx]
}

// SetSelectedInstance sets the selected index. Noop if the index is out of bounds.
func (l *List) SetSelectedInstance(idx int) {
	visibleItems := l.GetVisibleInstances()
	if idx >= len(visibleItems) {
		return
	}
	l.selectedIdx = idx
}

// RemoveSelectedFromView adjusts the selection after archiving/unarchiving
// This doesn't remove the instance, just adjusts the view
func (l *List) RemoveSelectedFromView() {
	visibleItems := l.GetVisibleInstances()
	if l.selectedIdx >= len(visibleItems)-1 && l.selectedIdx > 0 {
		l.selectedIdx--
	}
}

// GetInstances returns all instances in the list
func (l *List) GetInstances() []*session.Instance {
	return l.items
}

// GetVisibleInstances returns instances based on the current filter mode
func (l *List) GetVisibleInstances() []*session.Instance {
	var visible []*session.Instance
	for _, item := range l.items {
		switch l.filterMode {
		case FilterAll:
			if !item.Archived {
				visible = append(visible, item)
			}
		case FilterNeedsAttention:
			if !item.Archived && item.Status == session.Ready {
				visible = append(visible, item)
			}
		case FilterArchived:
			if item.Archived {
				visible = append(visible, item)
			}
		}
	}
	return visible
}

// NextFilter advances to the next filter mode (cycles through ALL -> NEEDS ATTENTION -> ARCHIVED -> ALL)
func (l *List) NextFilter() {
	l.filterMode = (l.filterMode + 1) % 3
	l.selectedIdx = 0  // Reset selection when filter changes
	l.scrollOffset = 0 // Reset scroll when filter changes
}

// PrevFilter goes to the previous filter mode (cycles backwards)
func (l *List) PrevFilter() {
	if l.filterMode == 0 {
		l.filterMode = FilterArchived
	} else {
		l.filterMode--
	}
	l.selectedIdx = 0  // Reset selection when filter changes
	l.scrollOffset = 0 // Reset scroll when filter changes
}

// GetFilterName returns a human-readable name for the current filter
func (l *List) GetFilterName() string {
	switch l.filterMode {
	case FilterNeedsAttention:
		return "NEEDS ATTENTION"
	case FilterArchived:
		return "ARCHIVED"
	default:
		return "ALL"
	}
}

// getFilterCounts returns counts for all, needs attention, and archived
func (l *List) getFilterCounts() (all, attention, archived int) {
	for _, item := range l.items {
		if item.Archived {
			archived++
		} else {
			all++
			if item.Status == session.Ready {
				attention++
			}
		}
	}
	return
}

// renderFilterTabs renders the filter tabs with counts
func (l *List) renderFilterTabs() string {
	allCount, attentionCount, archivedCount := l.getFilterCounts()

	var tabs []string

	// ALL tab
	allLabel := fmt.Sprintf("ALL(%d)", allCount)
	if l.filterMode == FilterAll {
		tabs = append(tabs, filterActiveStyle.Render(allLabel))
	} else {
		tabs = append(tabs, filterInactiveStyle.Render(allLabel))
	}

	// ATTENTION tab
	attentionLabel := fmt.Sprintf("ATTENTION(%d)", attentionCount)
	if l.filterMode == FilterNeedsAttention {
		tabs = append(tabs, filterActiveStyle.Render(attentionLabel))
	} else {
		tabs = append(tabs, filterInactiveStyle.Render(attentionLabel))
	}

	// ARCHIVED tab
	archivedLabel := fmt.Sprintf("ARCHIVED(%d)", archivedCount)
	if l.filterMode == FilterArchived {
		tabs = append(tabs, filterActiveStyle.Render(archivedLabel))
	} else {
		tabs = append(tabs, filterInactiveStyle.Render(archivedLabel))
	}

	// Join with arrows
	separator := filterStyle.Render(" ◀ ")
	return " " + strings.Join(tabs, separator) + filterStyle.Render(" ▶")
}

// ShowingArchived returns true if currently showing archived instances
func (l *List) ShowingArchived() bool {
	return l.filterMode == FilterArchived
}

// MergeInstances merges instances loaded from disk with the current in-memory instances.
// Merge strategy:
// - Instances in diskInstances but not in memory: Add them
// - Instances in memory but not in diskInstances: Keep if session alive, remove if dead
// - Instances in both: Keep in-memory version (more current)
// Returns true if any changes were made.
func (l *List) MergeInstances(diskInstances []*session.Instance) bool {
	// Build a map of current in-memory instances by title
	memoryMap := make(map[string]*session.Instance)
	for _, inst := range l.items {
		memoryMap[inst.Title] = inst
	}

	// Build a map of disk instances by title
	diskMap := make(map[string]*session.Instance)
	for _, inst := range diskInstances {
		diskMap[inst.Title] = inst
	}

	changed := false

	// Find instances to add (in disk but not in memory)
	for title, diskInst := range diskMap {
		if _, exists := memoryMap[title]; !exists {
			// Add this instance
			l.items = append(l.items, diskInst)
			// Register the repo
			repoName, err := diskInst.RepoName()
			if err == nil {
				l.addRepo(repoName)
			}
			log.InfoLog.Printf("Added instance from disk: %s", title)
			changed = true
		}
	}

	// Find instances to remove (in memory but not in disk, and session not alive)
	newItems := make([]*session.Instance, 0, len(l.items))
	for _, memInst := range l.items {
		if _, existsOnDisk := diskMap[memInst.Title]; existsOnDisk {
			// Instance exists on disk, keep it
			newItems = append(newItems, memInst)
		} else {
			// Instance not on disk - was it deleted by another process?
			if memInst.SessionAlive() {
				// Session is still running, keep it (don't kill running sessions)
				newItems = append(newItems, memInst)
				log.InfoLog.Printf("Keeping running instance not on disk: %s", memInst.Title)
			} else {
				// Session is dead/paused and not on disk, remove it
				repoName, err := memInst.RepoName()
				if err == nil {
					l.rmRepo(repoName)
				}
				log.InfoLog.Printf("Removed instance deleted from disk: %s", memInst.Title)
				changed = true
			}
		}
	}

	if changed {
		l.items = newItems
		// Adjust selected index if needed
		if l.selectedIdx >= len(l.items) {
			l.selectedIdx = max(0, len(l.items)-1)
		}
	}

	return changed
}
