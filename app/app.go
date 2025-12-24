package app

import (
	"claude-squad/config"
	"claude-squad/keys"
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/session/zellij"
	"claude-squad/ui"
	"claude-squad/ui/overlay"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const GlobalInstanceLimit = 50

// Run is the main entrypoint into the application.
func Run(ctx context.Context, program string, autoYes bool) error {
	p := tea.NewProgram(
		newHome(ctx, program, autoYes),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // Mouse scroll
	)
	_, err := p.Run()
	return err
}

type state int

const (
	stateDefault state = iota
	// stateNew is the state when the user is creating a new instance.
	stateNew
	// statePrompt is the state when the user is entering a prompt.
	statePrompt
	// stateHelp is the state when a help screen is displayed.
	stateHelp
	// stateConfirm is the state when a confirmation modal is displayed.
	stateConfirm
	// stateLoading is the state when a loading operation is in progress.
	stateLoading
	// stateRename is the state when the user is renaming an instance.
	stateRename
)

type home struct {
	ctx context.Context

	// -- Storage and Configuration --

	program string
	autoYes bool

	// storage is the interface for saving/loading data to/from the app's state
	storage *session.Storage
	// appConfig stores persistent application configuration
	appConfig *config.Config
	// appState stores persistent application state like seen help screens
	appState config.AppState

	// -- State --

	// state is the current discrete state of the application
	state state
	// newInstanceFinalizer is called when the state is stateNew and then you press enter.
	// It registers the new instance in the list after the instance has been started.
	newInstanceFinalizer func()

	// promptAfterName tracks if we should enter prompt mode after naming
	promptAfterName bool

	// pendingSave indicates that a save is queued (for debouncing)
	pendingSave bool

	// -- UI Components --

	// list displays the list of instances
	list *ui.List
	// menu displays the bottom menu
	menu *ui.Menu
	// tabbedWindow displays the tabbed window with preview and diff panes
	tabbedWindow *ui.TabbedWindow
	// errBox displays error messages
	errBox *ui.ErrBox
	// global spinner instance. we plumb this down to where it's needed
	spinner spinner.Model
	// textInputOverlay handles text input with state
	textInputOverlay *overlay.TextInputOverlay
	// textOverlay displays text information
	textOverlay *overlay.TextOverlay
	// confirmationOverlay displays confirmation modals
	confirmationOverlay *overlay.ConfirmationOverlay
	// loadingOverlay displays loading progress
	loadingOverlay *overlay.LoadingOverlay

	// -- Background Services --

	// summarizer handles generating AI summaries for instances
	summarizer *session.Summarizer
}

func newHome(ctx context.Context, program string, autoYes bool) *home {
	// Load application config
	appConfig := config.LoadConfig()

	// Load application state
	appState := config.LoadState()

	// Initialize storage
	storage, err := session.NewStorage(appState)
	if err != nil {
		fmt.Printf("Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}

	h := &home{
		ctx:          ctx,
		spinner:      spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane()),
		errBox:       ui.NewErrBox(),
		storage:      storage,
		appConfig:    appConfig,
		program:      program,
		autoYes:      autoYes,
		state:        stateDefault,
		appState:     appState,
		summarizer:   session.NewSummarizer(),
	}
	h.list = ui.NewList(&h.spinner, autoYes)

	// Load saved instances
	instances, err := storage.LoadInstances()
	if err != nil {
		fmt.Printf("Failed to load instances: %v\n", err)
		os.Exit(1)
	}

	// Add loaded instances to the list
	for _, instance := range instances {
		// Call the finalizer immediately.
		h.list.AddInstance(instance)()
		if autoYes {
			instance.AutoYes = true
		}
	}

	return h
}

// updateHandleWindowSizeEvent sets the sizes of the components.
// The components will try to render inside their bounds.
func (m *home) updateHandleWindowSizeEvent(msg tea.WindowSizeMsg) {
	// List takes 30% of width, preview takes 70%
	listWidth := int(float32(msg.Width) * 0.3)
	tabsWidth := msg.Width - listWidth

	// Menu takes 10% of height, list and window take 90%
	contentHeight := int(float32(msg.Height) * 0.9)
	menuHeight := msg.Height - contentHeight - 1     // minus 1 for error box
	m.errBox.SetSize(int(float32(msg.Width)*0.9), 1) // error box takes 1 row

	m.tabbedWindow.SetSize(tabsWidth, contentHeight)
	m.list.SetSize(listWidth, contentHeight)

	if m.textInputOverlay != nil {
		m.textInputOverlay.SetSize(int(float32(msg.Width)*0.6), int(float32(msg.Height)*0.4))
	}
	if m.textOverlay != nil {
		m.textOverlay.SetWidth(int(float32(msg.Width) * 0.6))
	}

	previewWidth, previewHeight := m.tabbedWindow.GetPreviewSize()
	if err := m.list.SetSessionPreviewSize(previewWidth, previewHeight); err != nil {
		log.ErrorLog.Print(err)
	}
	m.menu.SetSize(msg.Width, menuHeight)
}

func (m *home) Init() tea.Cmd {
	// Upon starting, we want to start the spinner. Whenever we get a spinner.TickMsg, we
	// update the spinner, which sends a new spinner.TickMsg. I think this lasts forever lol.
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			time.Sleep(100 * time.Millisecond)
			return previewTickMsg{}
		},
		tickUpdateMetadataCmd,
		tickUpdateSummaryCmd,
	)
}

func (m *home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case hideErrMsg:
		m.errBox.Clear()
	case previewTickMsg:
		cmd := m.instanceChanged()
		return m, tea.Batch(
			cmd,
			func() tea.Msg {
				time.Sleep(250 * time.Millisecond)
				return previewTickMsg{}
			},
		)
	case keyupMsg:
		m.menu.ClearKeydown()
		return m, nil
	case saveDebounceMsg:
		m.pendingSave = false
		// Perform the save asynchronously to avoid blocking the UI
		go func() {
			if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
				log.ErrorLog.Printf("failed to save instances: %v", err)
			}
		}()
		return m, nil
	case tickUpdateMetadataMessage:
		// Check if state file was modified by another process and sync if needed
		diskInstances, synced, err := m.storage.SyncFromDisk()
		if err != nil {
			log.WarningLog.Printf("failed to sync from disk: %v", err)
		} else if synced && diskInstances != nil {
			// Merge disk instances with in-memory instances
			if m.list.MergeInstances(diskInstances) {
				// If instances changed, update the UI
				m.instanceChanged()
			}
		}

		instances := m.list.GetInstances()

		// Parallel update check - runs HasUpdated() concurrently
		updateResults := session.ParallelUpdate(instances)
		for _, result := range updateResults {
			if result.Instance == nil {
				continue
			}
			if result.Updated {
				result.Instance.SetStatus(session.Running)
			} else {
				if result.HasPrompt {
					result.Instance.TapEnter()
				} else {
					result.Instance.SetStatus(session.Ready)
				}
			}
		}

		// Parallel diff stats update
		diffErrors := session.ParallelUpdateDiffStats(instances)
		for i, err := range diffErrors {
			if err != nil && instances[i] != nil {
				log.WarningLog.Printf("could not update diff stats: %v", err)
			}
		}

		return m, tickUpdateMetadataCmd
	case tickUpdateSummaryMessage:
		// Update the next instance's summary (staggered)
		instances := m.list.GetInstances()
		if updated := m.summarizer.UpdateNextSummary(instances); updated != nil {
			log.InfoLog.Printf("Updated summary for %s: %s", updated.Title, updated.Summary)
		}
		return m, tickUpdateSummaryCmd
	case loadingProgressMsg:
		if m.loadingOverlay != nil {
			m.loadingOverlay.SetStatus(msg.status)
		}
		return m, nil
	case loadingCompleteMsg:
		m.loadingOverlay = nil
		if msg.err != nil {
			m.list.Kill()
			m.state = stateDefault
			return m, m.handleError(msg.err)
		}
		// Instance started successfully
		instance := m.list.GetInstances()[m.list.NumInstances()-1]
		// Save after adding new instance
		if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
			return m, m.handleError(err)
		}
		// Instance added successfully, call the finalizer.
		m.newInstanceFinalizer()
		if m.autoYes {
			instance.AutoYes = true
		}

		m.newInstanceFinalizer()
		m.state = stateDefault
		if m.promptAfterName {
			m.state = statePrompt
			m.menu.SetState(ui.StatePrompt)
			// Initialize the text input overlay
			m.textInputOverlay = overlay.NewTextInputOverlay("Enter prompt", "")
			m.promptAfterName = false
		} else {
			m.menu.SetState(ui.StateDefault)
			m.showHelpScreen(helpStart(instance), nil)
		}

		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
	case tea.MouseMsg:
		// Handle mouse wheel events for scrolling the diff/preview pane
		if msg.Action == tea.MouseActionPress {
			if msg.Button == tea.MouseButtonWheelDown || msg.Button == tea.MouseButtonWheelUp {
				selected := m.list.GetSelectedInstance()
				if selected == nil || selected.Status == session.Paused {
					return m, nil
				}

				switch msg.Button {
				case tea.MouseButtonWheelUp:
					m.tabbedWindow.ScrollUp()
				case tea.MouseButtonWheelDown:
					m.tabbedWindow.ScrollDown()
				}
			}
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	case tea.WindowSizeMsg:
		m.updateHandleWindowSizeEvent(msg)
		return m, nil
	case error:
		// Handle errors from confirmation actions
		return m, m.handleError(msg)
	case instanceChangedMsg:
		// Handle instance changed after confirmation action
		return m, m.instanceChanged()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *home) handleQuit() (tea.Model, tea.Cmd) {
	if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
		return m, m.handleError(err)
	}
	return m, tea.Quit
}

// handleMenuHighlighting returns a command to highlight the pressed key in the menu.
// This is purely visual - it briefly underlines the corresponding menu item.
func (m *home) handleMenuHighlighting(msg tea.KeyMsg) tea.Cmd {
	if m.state == statePrompt || m.state == stateHelp || m.state == stateConfirm {
		return nil
	}
	// If it's in the global keymap, we should try to highlight it.
	name, ok := keys.GlobalKeyStringsMap[msg.String()]
	if !ok {
		return nil
	}

	if m.list.GetSelectedInstance() != nil && m.list.GetSelectedInstance().Paused() && name == keys.KeyEnter {
		return nil
	}
	if name == keys.KeyShiftDown || name == keys.KeyShiftUp {
		return nil
	}

	// TODO: cleanup: when you press enter on stateNew, we use keys.KeySubmitName. We should unify the keymap.
	if name == keys.KeyEnter && m.state == stateNew {
		name = keys.KeySubmitName
	}
	return m.keydownCallback(name)
}

func (m *home) handleKeyPress(msg tea.KeyMsg) (mod tea.Model, cmd tea.Cmd) {
	// Get the menu highlight command - this is batched with the action command later
	highlightCmd := m.handleMenuHighlighting(msg)

	if m.state == stateHelp {
		return m.handleHelpState(msg)
	}

	if m.state == stateNew {
		// Handle quit commands first. Don't handle q because the user might want to type that.
		if msg.String() == "ctrl+c" {
			m.state = stateDefault
			m.promptAfterName = false
			m.list.Kill()
			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		}

		instance := m.list.GetInstances()[m.list.NumInstances()-1]
		switch msg.Type {
		// Start the instance (enable previews etc) and go back to the main menu state.
		case tea.KeyEnter:
			if len(instance.Title) == 0 {
				return m, m.handleError(fmt.Errorf("title cannot be empty"))
			}

			// Show loading overlay and start instance in background
			m.loadingOverlay = overlay.NewLoadingOverlay("Creating Instance", &m.spinner)
			m.loadingOverlay.SetWidth(50)
			m.loadingOverlay.SetStatus("Initializing...")
			m.state = stateLoading

			// Start instance in a goroutine and send progress messages
			return m, m.startInstanceAsync(instance)
		case tea.KeyRunes:
			if len(instance.Title) >= 32 {
				return m, m.handleError(fmt.Errorf("title cannot be longer than 32 characters"))
			}
			if err := instance.SetTitle(instance.Title + string(msg.Runes)); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyBackspace:
			if len(instance.Title) == 0 {
				return m, nil
			}
			if err := instance.SetTitle(instance.Title[:len(instance.Title)-1]); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeySpace:
			if err := instance.SetTitle(instance.Title + " "); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyEsc:
			m.list.Kill()
			m.state = stateDefault
			m.instanceChanged()

			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		default:
		}
		return m, nil
	} else if m.state == statePrompt {
		// Use the new TextInputOverlay component to handle all key events
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)

		// Check if the form was submitted or canceled
		if shouldClose {
			selected := m.list.GetSelectedInstance()
			// TODO: this should never happen since we set the instance in the previous state.
			if selected == nil {
				return m, nil
			}
			if m.textInputOverlay.IsSubmitted() {
				if err := selected.SendPrompt(m.textInputOverlay.GetValue()); err != nil {
					// TODO: we probably end up in a bad state here.
					return m, m.handleError(err)
				}
			}

			// Close the overlay and reset state
			m.textInputOverlay = nil
			m.state = stateDefault
			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					m.showHelpScreen(helpStart(selected), nil)
					return nil
				},
			)
		}

		return m, nil
	} else if m.state == stateRename {
		// Use the TextInputOverlay component to handle key events for renaming
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)

		// Check if the form was submitted or canceled
		if shouldClose {
			selected := m.list.GetSelectedInstance()
			if selected == nil {
				m.textInputOverlay = nil
				m.state = stateDefault
				m.menu.SetState(ui.StateDefault)
				return m, nil
			}
			if m.textInputOverlay.IsSubmitted() {
				newTitle := m.textInputOverlay.GetValue()
				if err := selected.Rename(newTitle); err != nil {
					m.textInputOverlay = nil
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					return m, m.handleError(err)
				}
				// Save the updated instance
				if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
					m.textInputOverlay = nil
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					return m, m.handleError(err)
				}
			}

			// Close the overlay and reset state
			m.textInputOverlay = nil
			m.state = stateDefault
			return m, tea.Batch(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
				m.instanceChanged(),
			)
		}

		return m, nil
	}

	// Handle confirmation state
	if m.state == stateConfirm {
		shouldClose := m.confirmationOverlay.HandleKeyPress(msg)
		if shouldClose {
			m.state = stateDefault
			m.confirmationOverlay = nil
			return m, nil
		}
		return m, nil
	}

	// Exit scrolling mode when ESC is pressed and preview pane is in scrolling mode
	// Check if Escape key was pressed and we're not in the diff tab (meaning we're in preview tab)
	// Always check for escape key first to ensure it doesn't get intercepted elsewhere
	if msg.Type == tea.KeyEsc {
		// If in preview tab and in scroll mode, exit scroll mode
		if !m.tabbedWindow.IsInDiffTab() && m.tabbedWindow.IsPreviewInScrollMode() {
			// Use the selected instance from the list
			selected := m.list.GetSelectedInstance()
			err := m.tabbedWindow.ResetPreviewToNormalMode(selected)
			if err != nil {
				return m, m.handleError(err)
			}
			return m, m.instanceChanged()
		}
	}

	// Handle quit commands first
	if msg.String() == "ctrl+c" || msg.String() == "q" {
		return m.handleQuit()
	}

	name, ok := keys.GlobalKeyStringsMap[msg.String()]
	if !ok {
		return m, nil
	}

	switch name {
	case keys.KeyHelp:
		return m.showHelpScreen(helpTypeGeneral{}, nil)
	case keys.KeyPrompt:
		if m.list.NumInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:       "",
			Path:        ".",
			Program:     m.program,
			Multiplexer: m.appConfig.Multiplexer,
		})
		if err != nil {
			return m, m.handleError(err)
		}

		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)
		m.promptAfterName = true

		return m, nil
	case keys.KeyNew:
		if m.list.NumInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:       "",
			Path:        ".",
			Program:     m.program,
			Multiplexer: m.appConfig.Multiplexer,
		})
		if err != nil {
			return m, m.handleError(err)
		}

		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)

		return m, nil
	case keys.KeyUp:
		m.list.Up()
		return m, tea.Batch(highlightCmd, m.instanceChanged())
	case keys.KeyDown:
		m.list.Down()
		return m, tea.Batch(highlightCmd, m.instanceChanged())
	case keys.KeyShiftUp:
		m.tabbedWindow.ScrollUp()
		return m, tea.Batch(highlightCmd, m.instanceChanged())
	case keys.KeyShiftDown:
		m.tabbedWindow.ScrollDown()
		return m, tea.Batch(highlightCmd, m.instanceChanged())
	case keys.KeyMoveUp:
		if m.list.MoveUp() {
			// Schedule debounced save to avoid blocking on rapid key presses
			return m, tea.Batch(highlightCmd, m.instanceChanged(), m.requestSave())
		}
		return m, tea.Batch(highlightCmd, m.instanceChanged())
	case keys.KeyMoveDown:
		if m.list.MoveDown() {
			// Schedule debounced save to avoid blocking on rapid key presses
			return m, tea.Batch(highlightCmd, m.instanceChanged(), m.requestSave())
		}
		return m, tea.Batch(highlightCmd, m.instanceChanged())
	case keys.KeyToggleArchive:
		m.list.ToggleArchiveView()
		m.menu.SetShowingArchived(m.list.ShowingArchived())
		return m, tea.Batch(highlightCmd, m.instanceChanged())
	case keys.KeyArchive:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Determine action based on current view and instance state
		if m.list.ShowingArchived() {
			// In archived view - unarchive (restore) the instance
			restoreAction := func() tea.Msg {
				selected.Archived = false
				if err := m.storage.UnarchiveInstance(selected.Title); err != nil {
					return err
				}
				m.list.RemoveSelectedFromView()
				return instanceChangedMsg{}
			}
			message := fmt.Sprintf("[!] Restore session '%s'?", selected.Title)
			return m, m.confirmAction(message, restoreAction)
		} else {
			// In active view - archive the instance
			archiveAction := func() tea.Msg {
				// Pause the instance first if it's running
				if !selected.Paused() {
					if err := selected.Pause(); err != nil {
						return err
					}
				}
				selected.Archived = true
				if err := m.storage.ArchiveInstance(selected.Title); err != nil {
					return err
				}
				m.list.RemoveSelectedFromView()
				return instanceChangedMsg{}
			}
			message := fmt.Sprintf("[!] Archive session '%s'?", selected.Title)
			return m, m.confirmAction(message, archiveAction)
		}
	case keys.KeyTab:
		m.tabbedWindow.Toggle()
		m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
		return m, tea.Batch(highlightCmd, m.instanceChanged())
	case keys.KeyKill:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Create the kill action as a tea.Cmd
		killAction := func() tea.Msg {
			// Get worktree and check if branch is checked out
			worktree, err := selected.GetGitWorktree()
			if err != nil {
				return err
			}

			checkedOut, err := worktree.IsBranchCheckedOut()
			if err != nil {
				return err
			}

			if checkedOut {
				return fmt.Errorf("instance %s is currently checked out", selected.Title)
			}

			// Delete from storage first
			if err := m.storage.DeleteInstance(selected.Title); err != nil {
				return err
			}

			// Then kill the instance
			m.list.Kill()
			return instanceChangedMsg{}
		}

		// Show confirmation modal
		message := fmt.Sprintf("[!] Kill session '%s'?", selected.Title)
		return m, m.confirmAction(message, killAction)
	case keys.KeySubmit:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Create the push action as a tea.Cmd
		pushAction := func() tea.Msg {
			// Default commit message with timestamp
			commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s", selected.Title, time.Now().Format(time.RFC822))
			worktree, err := selected.GetGitWorktree()
			if err != nil {
				return err
			}
			if err = worktree.PushChanges(commitMsg, true); err != nil {
				return err
			}
			return nil
		}

		// Show confirmation modal
		message := fmt.Sprintf("[!] Push changes from session '%s'?", selected.Title)
		return m, m.confirmAction(message, pushAction)
	case keys.KeyCheckout:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Show help screen before pausing
		m.showHelpScreen(helpTypeInstanceCheckout{}, func() {
			if err := selected.Pause(); err != nil {
				m.handleError(err)
			}
			m.instanceChanged()
		})
		return m, nil
	case keys.KeyResume:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		if err := selected.Resume(); err != nil {
			return m, m.handleError(err)
		}
		return m, tea.WindowSize()
	case keys.KeyImport:
		return m.handleImportOrphanedSessions()
	case keys.KeyEnter:
		if m.list.NumInstances() == 0 {
			return m, nil
		}
		selected := m.list.GetSelectedInstance()
		if selected == nil || selected.Paused() || !selected.SessionAlive() {
			return m, nil
		}
		// Show help screen before attaching
		m.showHelpScreen(helpTypeInstanceAttach{}, func() {
			ch, err := m.list.Attach()
			if err != nil {
				m.handleError(err)
				return
			}
			<-ch
			m.state = stateDefault
		})
		return m, nil
	case keys.KeyRename:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		// Enter rename mode with the current title pre-filled
		m.state = stateRename
		m.menu.SetState(ui.StateRename)
		m.textInputOverlay = overlay.NewTextInputOverlay("Rename instance", selected.Title)
		return m, nil
	default:
		return m, nil
	}
}

// instanceChanged updates the preview pane, menu, and diff pane based on the selected instance. It returns an error
// Cmd if there was any error.
func (m *home) instanceChanged() tea.Cmd {
	// selected may be nil
	selected := m.list.GetSelectedInstance()

	m.tabbedWindow.UpdateDiff(selected)
	m.tabbedWindow.SetInstance(selected)
	// Update menu with current instance
	m.menu.SetInstance(selected)

	// If there's no selected instance, we don't need to update the preview.
	if err := m.tabbedWindow.UpdatePreview(selected); err != nil {
		return m.handleError(err)
	}
	return nil
}

type keyupMsg struct{}

// keydownCallback clears the menu option highlighting after 500ms.
func (m *home) keydownCallback(name keys.KeyName) tea.Cmd {
	m.menu.Keydown(name)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(500 * time.Millisecond):
		}

		return keyupMsg{}
	}
}

// hideErrMsg implements tea.Msg and clears the error text from the screen.
type hideErrMsg struct{}

// previewTickMsg implements tea.Msg and triggers a preview update
type previewTickMsg struct{}

type tickUpdateMetadataMessage struct{}

type tickUpdateSummaryMessage struct{}

type instanceChangedMsg struct{}

// saveDebounceMsg is sent after a debounce delay to trigger a save
type saveDebounceMsg struct{}

// saveDebounceDelay is how long to wait before saving after a reorder operation
const saveDebounceDelay = 500 * time.Millisecond

// loadingProgressMsg is sent when there's a progress update during loading
type loadingProgressMsg struct {
	status string
}

// loadingCompleteMsg is sent when the loading operation completes
type loadingCompleteMsg struct {
	err error
}

// tickUpdateMetadataCmd is the callback to update the metadata of the instances every 2 seconds.
// Note that we iterate over all instances and capture their output. It's an expensive operation.
var tickUpdateMetadataCmd = func() tea.Msg {
	time.Sleep(2 * time.Second)
	return tickUpdateMetadataMessage{}
}

// tickUpdateSummaryCmd is the callback to update instance summaries. This is staggered across instances
// so we don't overwhelm the system with Claude CLI calls.
var tickUpdateSummaryCmd = func() tea.Msg {
	time.Sleep(session.SummaryRefreshInterval)
	return tickUpdateSummaryMessage{}
}

// handleError handles all errors which get bubbled up to the app. sets the error message. We return a callback tea.Cmd that returns a hideErrMsg message
// which clears the error message after 3 seconds.
func (m *home) handleError(err error) tea.Cmd {
	log.ErrorLog.Printf("%v", err)
	m.errBox.SetError(err)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(3 * time.Second):
		}

		return hideErrMsg{}
	}
}

// requestSave schedules a debounced save operation.
// If a save is already pending, this does nothing (the pending save will include all changes).
func (m *home) requestSave() tea.Cmd {
	if m.pendingSave {
		return nil // Already have a pending save
	}
	m.pendingSave = true
	return func() tea.Msg {
		time.Sleep(saveDebounceDelay)
		return saveDebounceMsg{}
	}
}

// startInstanceAsync starts an instance in a goroutine and returns a tea.Cmd that
// sends a completion message when done.
func (m *home) startInstanceAsync(instance *session.Instance) tea.Cmd {
	return func() tea.Msg {
		// Start the instance with a progress callback that updates the overlay
		// Note: Progress updates happen synchronously during Start(), so we can
		// update the overlay directly via the callback
		err := instance.StartWithProgress(true, func(status string) {
			if m.loadingOverlay != nil {
				m.loadingOverlay.SetStatus(status)
			}
		})
		return loadingCompleteMsg{err: err}
	}
}

// confirmAction shows a confirmation modal and stores the action to execute on confirm
func (m *home) confirmAction(message string, action tea.Cmd) tea.Cmd {
	m.state = stateConfirm

	// Create and show the confirmation overlay using ConfirmationOverlay
	m.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	// Set a fixed width for consistent appearance
	m.confirmationOverlay.SetWidth(50)

	// Set callbacks for confirmation and cancellation
	m.confirmationOverlay.OnConfirm = func() {
		m.state = stateDefault
		// Execute the action if it exists
		if action != nil {
			_ = action()
		}
	}

	m.confirmationOverlay.OnCancel = func() {
		m.state = stateDefault
	}

	return nil
}

// handleImportOrphanedSessions finds and imports orphaned Zellij sessions
func (m *home) handleImportOrphanedSessions() (tea.Model, tea.Cmd) {
	// Get list of currently tracked instance titles
	instances := m.list.GetInstances()
	trackedTitles := make([]string, len(instances))
	for i, inst := range instances {
		trackedTitles[i] = inst.Title
	}

	// Find orphaned sessions
	orphans, err := zellij.ListOrphanedSessions(trackedTitles, nil)
	if err != nil {
		return m, m.handleError(fmt.Errorf("failed to list orphaned sessions: %w", err))
	}

	if len(orphans) == 0 {
		return m, m.handleError(fmt.Errorf("no orphaned sessions found"))
	}

	// Check instance limit
	if m.list.NumInstances()+len(orphans) > GlobalInstanceLimit {
		return m, m.handleError(fmt.Errorf("importing %d sessions would exceed the limit of %d instances",
			len(orphans), GlobalInstanceLimit))
	}

	// Import each orphaned session
	importedCount := 0
	var importErrors []string
	for _, orphan := range orphans {
		// Recover full metadata for this session
		recovered, err := zellij.RecoverMetadata(orphan.SessionName, nil)
		if err != nil {
			importErrors = append(importErrors, fmt.Sprintf("%s: %v", orphan.Title, err))
			continue
		}

		// Create instance from recovered data
		instance, err := session.NewInstanceFromOrphan(recovered)
		if err != nil {
			importErrors = append(importErrors, fmt.Sprintf("%s: %v", orphan.Title, err))
			continue
		}

		// Add to the list
		finalizer := m.list.AddInstance(instance)
		finalizer()
		if m.autoYes {
			instance.AutoYes = true
		}
		importedCount++
	}

	// Save state after importing
	if importedCount > 0 {
		if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
			return m, m.handleError(fmt.Errorf("failed to save after import: %w", err))
		}
	}

	// Report results
	if len(importErrors) > 0 {
		log.WarningLog.Printf("Import errors: %v", importErrors)
		if importedCount == 0 {
			return m, m.handleError(fmt.Errorf("failed to import any sessions"))
		}
		return m, m.handleError(fmt.Errorf("imported %d session(s), %d failed", importedCount, len(importErrors)))
	}

	// Show success message via error box (it's just a message display)
	m.errBox.SetError(fmt.Errorf("imported %d orphaned session(s)", importedCount))
	return m, tea.Batch(
		tea.WindowSize(),
		m.instanceChanged(),
		func() tea.Msg {
			time.Sleep(3 * time.Second)
			return hideErrMsg{}
		},
	)
}

func (m *home) View() string {
	listWithPadding := lipgloss.NewStyle().PaddingTop(1).Render(m.list.String())
	previewWithPadding := lipgloss.NewStyle().PaddingTop(1).Render(m.tabbedWindow.String())
	listAndPreview := lipgloss.JoinHorizontal(lipgloss.Top, listWithPadding, previewWithPadding)

	mainView := lipgloss.JoinVertical(
		lipgloss.Center,
		listAndPreview,
		m.menu.String(),
		m.errBox.String(),
	)

	if m.state == statePrompt || m.state == stateRename {
		if m.textInputOverlay == nil {
			log.ErrorLog.Printf("text input overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	} else if m.state == stateHelp {
		if m.textOverlay == nil {
			log.ErrorLog.Printf("text overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.textOverlay.Render(), mainView, true, true)
	} else if m.state == stateConfirm {
		if m.confirmationOverlay == nil {
			log.ErrorLog.Printf("confirmation overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.confirmationOverlay.Render(), mainView, true, true)
	} else if m.state == stateLoading {
		if m.loadingOverlay == nil {
			log.ErrorLog.Printf("loading overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.loadingOverlay.Render(), mainView, true, true)
	}

	return mainView
}
