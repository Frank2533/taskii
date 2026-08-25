package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"taskii/internal/deadline"
	"taskii/internal/model"
	"taskii/internal/obsidian"
	"taskii/internal/para"
	"taskii/internal/stats"
	"taskii/internal/worklog"
)

type focusedPane int

const (
	focusToday focusedPane = iota
	focusOverdue
	focusNotes
	focusReports

	focusCount = 4
)

type mode int

const (
	modeNormal mode = iota
	modeAdding
	modeConfirmDelete
	modeNoteEditing
	modeConfirmClearNotes
	// modeVaultAdding reuses the task input to append a checkbox to a note in
	// the vault rather than creating a local task.
	modeVaultAdding
	// modeTicketNote reuses the input to append a dated note to a ticket's
	// freeform work-log section.
	modeTicketNote
	// modeEditRow edits the task or subtask under the cursor in place.
	modeEditRow
	// modeAddSubtask adds a task line to the selected ticket's note.
	modeAddSubtask
)

const dateFormat = "2006-01-02"

type App struct {
	tasks []model.Task
	now   func() time.Time

	width, height int

	focus focusedPane
	mode  mode

	todaySelected int
	todayScroll   int

	overdueSelected int
	overdueScroll   int

	filterImportant bool
	filterUndone    bool

	pomo pomodoro

	notes         []model.Note
	notesSelected int
	notesScroll   int
	noteInput     textarea.Model
	noteEditIndex int // -1 when adding, otherwise the note being edited
	// notesExpanded blows the Notes board up to the whole body area, hiding
	// every other pane. Deliberately NOT persisted: it's a transient working
	// state, not a preference like theme or layout.
	notesExpanded bool

	// reportChart is which chart the Reports pane shows; navigable with the
	// arrow keys while that pane is focused.
	reportChart reportChart

	// Simple mode: one pane, one merged list. simpleNoteInput selects which
	// input tab is active (tab toggles between adding a task and a note).
	simple         bool
	simpleSelected int
	simpleScroll   int
	simpleNoteMode bool

	input     textinput.Model
	err       string
	status    string
	noPersist bool

	username string
	layout   layout

	// --- vault-backed views ---

	// view is the active screen. Panes are composed per view; see paraview.go.
	view view

	// vaultPath is the indexed vault, "" when none is configured. The app
	// still runs: the dashboard needs no vault at all.
	vaultPath string

	// loc governs every date boundary in the app. Set from settings at
	// startup and never nil, so no date math has to reach for time.Local.
	loc *time.Location

	// obs drives the Obsidian CLI. Non-nil even when the CLI is missing, so
	// actions can explain themselves instead of silently doing nothing.
	obs *obsidian.Client

	idx    *para.Index
	idxErr error

	// vault holds the PARA view's selection and scroll state.
	vault vaultState

	// wl accrues pomodoro time against issue keys, locally.
	wl *worklog.Log

	// pomoKey is the ticket the running timer is credited to, "" when the
	// timer is not bound to anything.
	pomoKey string

	// pomoCounted is how much of the current work phase has already been
	// banked, so each tick credits only the delta.
	pomoCounted time.Duration

	icsOut   string
	icsEvery time.Duration

	// busy names the in-flight CLI action, "" when idle.
	busy string

	settingsUI settingsState

	// reminderBanner holds the most recent start-reminder text so it stays
	// visible in the app as well as on the desktop; a desktop notification
	// disappears whether or not it was seen.
	reminderBanner   string
	reminderBannerAt time.Time

	// addSuggest is the highlighted ticket in the add box's typeahead, or -1
	// when the text will be taken as a plain task. Adding a ticket is always
	// a deliberate selection, never something the typeahead does for you.
	addSuggest int

	// picker is the one-keystroke deadline chooser.
	picker deadlinePicker

	// editing is what an in-progress edit applies to.
	editing editTarget

	// showKeys is the "all bindings" overlay.
	showKeys bool

	// Raw persisted setting values, kept verbatim so an empty string keeps
	// meaning "derive this" rather than being frozen into whatever was
	// derived at startup.
	tzSetting          string
	vaultSetting       string
	icsSetting         string
	icsIntervalSetting string
	worklogPush        bool
	jiraEnabled        bool
	obsidianSync       bool
	projectFolder      string
}

// Options configures NewApp for non-default startup modes.
type Options struct {
	// Mock runs the app against generated sample data: no read from or write
	// to data/tasks.json, so a demo/screenshot run never touches real data.
	Mock bool
	// Vault is the Obsidian vault to index. Empty disables the vault-backed
	// views rather than preventing startup.
	Vault string
	// Location governs every date boundary. Never nil by the time NewApp
	// returns.
	Location *time.Location
	// Simple renders a single full-screen pane: the greeting in a fixed left
	// column, and one list merging today's tasks, overdue tasks and notes in
	// creation order beside it.
	Simple bool
}

func NewApp(opts Options) App {
	var tasks []model.Task
	errMsg := ""

	if opts.Mock {
		tasks = mockTasks(time.Now())
	} else {
		var err error
		tasks, err = model.Load()
		if err != nil {
			errMsg = "failed to load tasks: " + err.Error()
			tasks = []model.Task{}
		}
	}

	settings, _ := model.LoadSettings()
	lay := layoutTasksLeft
	if !opts.Mock {
		if settings.Theme != "" {
			setThemeByName(settings.Theme)
		}
		if settings.Layout != "" {
			lay = layoutByName(settings.Layout)
		}
	}

	loc := opts.Location
	if loc == nil {
		loc = settings.Location()
	}

	// Tracked time is a local record, so a mock run must not touch it.
	wl := &worklog.Log{Entries: map[string]*worklog.Entry{}}
	if !opts.Mock {
		if loaded, err := worklog.Load(); err == nil {
			wl = loaded
		}
	}

	var notes []model.Note
	if opts.Mock {
		notes = mockNotes()
	} else if n, err := model.LoadNotes(); err == nil {
		notes = n
	}

	ti := textinput.New()
	ti.Placeholder = "Title, or end with HH:MM to add it as an appointment"
	ti.CharLimit = 120

	ta := textarea.New()
	ta.Placeholder = "Note — ctrl+j or opt+enter for a new line"
	ta.ShowLineNumbers = false
	// No CharLimit: notes are explicitly unbounded in length.
	ta.CharLimit = 0
	// The textarea binds enter to "insert newline" by default; here enter
	// SAVES and shift/alt+enter inserts the newline, so that binding is
	// cleared and handled in updateNoteEditing instead.
	ta.KeyMap.InsertNewline.SetEnabled(false)

	icsOut := settings.ICSOutput
	if icsOut == "" && opts.Vault != "" {
		icsOut = filepath.Join(opts.Vault, "Calendar", "taskii.ics")
	}

	return App{
		tasks: tasks,
		// Every date the app shows is derived from this, so the configured
		// zone applies uniformly instead of each call site reaching for
		// time.Local.
		now:           func() time.Time { return time.Now().In(loc) },
		focus:         focusToday,
		mode:          modeNormal,
		input:         ti,
		notes:         notes,
		noteInput:     ta,
		noteEditIndex: -1,
		simple:        opts.Simple,
		err:           errMsg,
		pomo:          newPomodoro(),
		noPersist:     opts.Mock,
		username:      currentUsername(),
		layout:        lay,

		view: viewDashboard,
		// -1 means "no suggestion highlighted", so a bare Enter adds what was
		// typed rather than silently attaching it to the first match.
		addSuggest: -1,

		vaultPath: opts.Vault,
		loc:       loc,
		obs:       obsidian.New(opts.Vault),
		wl:        wl,

		tzSetting:          settings.Timezone,
		vaultSetting:       settings.VaultPath,
		icsSetting:         settings.ICSOutput,
		icsIntervalSetting: settings.ICSInterval,
		worklogPush:        settings.WorklogPushToJira,
		jiraEnabled:        settings.JiraEnabled(),
		obsidianSync:       settings.ObsidianSync,
		projectFolder:      settings.ProjectFolder,

		icsOut:   icsOut,
		icsEvery: settings.ICSEvery(),
	}
}

func (a App) Init() tea.Cmd {
	cmds := []tea.Cmd{pomodoroTick(), reminderTick()}
	// The vault is indexed off the UI goroutine so a large vault never delays
	// the first paint.
	if c := loadIndex(a.vaultPath, a.loc); c != nil {
		cmds = append(cmds, c)
	}
	if !a.noPersist {
		if c := icsTick(a.icsEvery); c != nil {
			cmds = append(cmds, c)
		}
	}
	return tea.Batch(cmds...)
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil

	case pomodoroTickMsg:
		phaseChanged := a.pomo.tick()
		a.creditPomodoro()
		if phaseChanged {
			return a, tea.Batch(pomodoroTick(), notifyPhaseChange(a.pomo.phase))
		}
		return a, pomodoroTick()

	case reminderTickMsg:
		return a, tea.Batch(a.fireReminders(), reminderTick())

	case reminderFiredMsg:
		a.reminderBanner = "Time to start: " + strings.Join(msg.titles, ", ")
		a.reminderBannerAt = a.now()
		return a, nil

	case indexMsg:
		a.idx, a.idxErr = msg.idx, msg.err
		a.clampVaultSelections()
		return a, nil

	case actionMsg:
		a.busy = ""
		if msg.err != nil {
			a.err = msg.label + ": " + msg.err.Error()
			return a, nil
		}
		a.status = msg.label + " done"
		// A plugin command may have rewritten the note, so re-read the vault
		// rather than trusting the index we already hold.
		return a, loadIndex(a.vaultPath, a.loc)

	case icsMsg:
		if msg.err != nil {
			a.err = "calendar export: " + msg.err.Error()
		} else if msg.wrote {
			a.status = fmt.Sprintf("calendar: %d events written", msg.count)
		}
		return a, nil

	case icsTickMsg:
		return a, tea.Batch(
			exportICS(a.vaultPath, a.icsOut, a.tasks, a.loc),
			icsTick(a.icsEvery),
		)

	case tea.KeyMsg:
		if a.settingsUI.open {
			return a.updateSettings(msg)
		}
		if a.picker.open {
			return a.updateDeadlinePicker(msg)
		}
		if a.showKeys {
			// Any key dismisses the reference, so it never traps the user.
			a.showKeys = false
			return a, nil
		}
		switch a.mode {
		case modeVaultAdding:
			return a.updateVaultAdding(msg)
		case modeTicketNote:
			return a.updateTicketNote(msg)
		case modeEditRow:
			return a.updateEditRow(msg)
		case modeAddSubtask:
			return a.updateAddSubtask(msg)
		case modeAdding:
			return a.updateAdding(msg)
		case modeConfirmDelete:
			return a.updateConfirmDelete(msg)
		case modeNoteEditing:
			return a.updateNoteEditing(msg)
		case modeConfirmClearNotes:
			return a.updateConfirmClearNotes(msg)
		}
		// View switching and settings are app-wide, but --simple is a
		// deliberately single-screen mode with no other views to reach.
		if !a.simple {
			switch msg.String() {
			case "1":
				a.view = viewDashboard
				return a, nil
			case "2":
				a.view = viewPARA
				return a, nil
			case "3":
				a.view = viewCalendar
				return a, nil
			case "?":
				a.showKeys = true
				return a, nil
			case ",":
				a.settingsUI.open = true
				a.settingsUI.err = ""
				return a, nil
			case "q", "ctrl+c":
				return a, tea.Quit
			}
			switch a.view {
			case viewPARA:
				return a.updatePara(msg)
			case viewCalendar:
				return a.updateCalendar(msg)
			}
		}
		return a.updateNormal(msg)
	}

	return a, nil
}

// updateCalendar is the calendar view's key map. It is intentionally small:
// the calendar is a read-only projection of the vault and local tasks.
func (a App) updateCalendar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "r":
		return a, loadIndex(a.vaultPath, a.loc)
	case "C":
		return a, exportICS(a.vaultPath, a.icsOut, a.tasks, a.loc)
	}
	return a, nil
}

// updateVaultAdding handles the quick-add input for a note checkbox.
func (a App) updateVaultAdding(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.mode = modeNormal
		a.input.Blur()
		a.input.SetValue("")
		return a, nil
	case "enter":
		return a.commitQuickAdd(a.input.Value())
	}
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return a, cmd
}

// clampVaultSelections keeps the PARA view's cursors inside the freshly
// rebuilt index.
func (a *App) clampVaultSelections() {
	if rows := a.treeRows(); len(rows) > 0 {
		a.vault.treeSel = clamp(a.vault.treeSel, 0, len(rows)-1)
	} else {
		a.vault.treeSel = 0
	}
	if list := a.visibleTickets(); len(list) > 0 {
		a.vault.listSel = clamp(a.vault.listSel, 0, len(list)-1)
	} else {
		a.vault.listSel = 0
	}
	if t, ok := a.selectedTicket(); ok && len(t.Checkboxes) > 0 {
		a.vault.detailSel = clamp(a.vault.detailSel, 0, len(t.Checkboxes)-1)
	} else {
		a.vault.detailSel = 0
	}
}

// expandedAllowedKeys are the only bindings that stay live while the Notes
// board is expanded: the notes keys themselves plus the app-wide ones. Every
// other pane is off-screen, so its keys would act invisibly — pressing `p`
// would start a Pomodoro nobody can see.
var expandedAllowedKeys = map[string]bool{
	// Notes.
	"a": true, "enter": true, "d": true, "C": true, "e": true,
	"up": true, "k": true, "down": true, "j": true,
	// App-wide.
	"q": true, "ctrl+c": true, "t": true, "L": true,
}

// updateSimple is the whole key map for --simple: one list, one selection,
// and tab choosing which kind of item `a` will add. The other panes don't
// exist here, so their bindings (pomodoro, filters, focus switching, layout)
// are simply absent rather than being gated off.
func (a App) updateSimple(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	entries := a.simpleEntries()

	switch msg.String() {
	case "q", "ctrl+c":
		return a, tea.Quit

	case "tab":
		a.simpleNoteMode = !a.simpleNoteMode
		return a, nil

	case "up", "k":
		a.moveSimpleSelection(-1, entries)
		return a, nil

	case "down", "j":
		a.moveSimpleSelection(1, entries)
		return a, nil

	case "a":
		if a.simpleNoteMode {
			return a.startNoteEdit(-1)
		}
		a.mode = modeAdding
		a.input.SetValue("")
		a.input.Focus()
		return a, textinput.Blink

	case " ", "enter":
		if a.simpleSelected < 0 || a.simpleSelected >= len(entries) {
			return a, nil
		}
		e := entries[a.simpleSelected]
		if e.isNote {
			if msg.String() == "enter" {
				return a.startNoteEdit(e.noteIndex)
			}
			return a, nil
		}
		a.toggleTaskByID(e.task.ID)
		return a, nil

	case "d":
		if a.simpleSelected >= 0 && a.simpleSelected < len(entries) {
			a.mode = modeConfirmDelete
		}
		return a, nil

	case "i":
		if a.simpleSelected >= 0 && a.simpleSelected < len(entries) {
			if e := entries[a.simpleSelected]; !e.isNote {
				a.toggleImportantByID(e.task.ID)
			}
		}
		return a, nil

	case "t":
		name := cycleTheme()
		a.status = "Theme: " + name
		a.saveSettings()
		return a, nil
	}
	return a, nil
}

func (a *App) moveSimpleSelection(delta int, entries []simpleEntry) {
	if len(entries) == 0 {
		return
	}
	sel := a.simpleSelected + delta
	if sel < 0 {
		sel = 0
	}
	if sel >= len(entries) {
		sel = len(entries) - 1
	}
	a.simpleSelected = sel
	a.syncSimpleScroll(entries)
}

// syncSimpleScroll keeps the selected entry's display rows in view. Same
// note-vs-line distinction as the notes board: selection counts entries,
// the scroll offset counts display lines.
func (a *App) syncSimpleScroll(entries []simpleEntry) {
	visible := a.simpleVisibleRows()
	all := a.simpleLines(entries, a.simpleListWidth())
	first, last := -1, -1
	for i, l := range all {
		if l.entryIndex == a.simpleSelected {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return
	}
	scroll := a.simpleScroll
	if first < scroll {
		scroll = first
	} else if last >= scroll+visible {
		scroll = last - visible + 1
		if scroll > first {
			scroll = first
		}
	}
	if scroll < 0 {
		scroll = 0
	}
	a.simpleScroll = scroll
}

func (a *App) toggleTaskByID(id string) {
	for i := range a.tasks {
		if a.tasks[i].ID == id {
			a.tasks[i].Done = !a.tasks[i].Done
			if a.tasks[i].Done {
				t := a.now()
				a.tasks[i].DoneAt = &t
			} else {
				a.tasks[i].DoneAt = nil
			}
			defer a.syncLocalTask(a.tasks[i].ID)
			a.persist()
			return
		}
	}
}

func (a *App) toggleImportantByID(id string) {
	for i := range a.tasks {
		if a.tasks[i].ID == id {
			a.tasks[i].Important = !a.tasks[i].Important
			a.persist()
			return
		}
	}
}

func (a *App) deleteSimpleSelected() {
	entries := a.simpleEntries()
	if a.simpleSelected < 0 || a.simpleSelected >= len(entries) {
		return
	}
	e := entries[a.simpleSelected]
	if e.isNote {
		if e.noteIndex >= 0 && e.noteIndex < len(a.notes) {
			a.notes = append(a.notes[:e.noteIndex], a.notes[e.noteIndex+1:]...)
			a.persistNotes()
		}
	} else {
		for i := range a.tasks {
			if a.tasks[i].ID == e.task.ID {
				a.tasks = append(a.tasks[:i], a.tasks[i+1:]...)
				a.persist()
				break
			}
		}
	}
	if a.simpleSelected >= len(a.simpleEntries()) {
		a.simpleSelected = len(a.simpleEntries()) - 1
	}
	if a.simpleSelected < 0 {
		a.simpleSelected = 0
	}
}

func (a App) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.simple {
		return a.updateSimple(msg)
	}
	if a.notesExpanded && !expandedAllowedKeys[msg.String()] {
		return a, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return a, tea.Quit

	case "tab":
		a.focus = (a.focus + 1) % focusCount
		a.clampSelections()
		return a, nil

	case "shift+tab":
		a.focus = (a.focus + focusCount - 1) % focusCount
		a.clampSelections()
		return a, nil

	case "up", "k", "left", "h":
		// On the Reports pane the navigation keys move between charts
		// rather than between rows — that pane has no list.
		if a.focus == focusReports {
			a.reportChart = a.reportChart.prev()
			return a, nil
		}
		a.moveSelection(-1)
		return a, nil

	case "down", "j", "right", "l":
		if a.focus == focusReports {
			a.reportChart = a.reportChart.next()
			return a, nil
		}
		a.moveSelection(1)
		return a, nil

	case "a":
		switch a.focus {
		case focusReports:
			// No items on this pane.
			return a, nil
		case focusToday:
			a.mode = modeAdding
			a.input.SetValue("")
			a.input.Focus()
			return a, textinput.Blink
		case focusNotes:
			return a.startNoteEdit(-1)
		}
		return a, nil

	case " ", "enter":
		if a.focus == focusReports {
			return a, nil
		}
		// On the notes board enter opens the selected note for editing;
		// on the task lists it toggles done.
		if a.focus == focusNotes {
			if msg.String() == "enter" && len(a.notes) > 0 {
				return a.startNoteEdit(a.notesSelected)
			}
			return a, nil
		}
		if a.focus == focusToday {
			// A ticket's subtasks live in the vault note, so toggling one
			// writes there rather than to any local copy — the note stays the
			// single source of truth and the two views cannot drift apart.
			next, reindex := a.toggleTodayRow()
			if reindex {
				return next, loadIndex(next.vaultPath, next.loc)
			}
			return next, nil
		}
		a.toggleSelected()
		return a, nil

	case "A":
		if a.focus == focusToday {
			return a.beginAddSubtask()
		}
		return a, nil

	case "D":
		// A picker, because typing a token per task is more ceremony than it
		// is worth when you are triaging a list.
		if a.focus == focusToday || a.focus == focusOverdue {
			a.picker.open = true
		}
		return a, nil

	case "z":
		// Fold a ticket row away without losing sight of its progress: the
		// collapsed row still reports its subtask count.
		if a.focus == focusToday {
			a.toggleCollapseTodayRow()
			a.clampSelections()
		}
		return a, nil

	case "d":
		// Deletion is irreversible (there's no undo), so it takes a second
		// keystroke. Only enter the mode if something is actually selected,
		// otherwise the prompt would ask about nothing.
		if a.focus == focusReports {
			return a, nil
		}
		if a.focus == focusNotes {
			if len(a.notes) > 0 {
				a.mode = modeConfirmDelete
			}
			return a, nil
		}
		if a.selectedTask() != nil {
			a.mode = modeConfirmDelete
		}
		return a, nil

	case "C":
		// Clear the whole board. Capitalised and confirmed, since it discards
		// everything at once.
		if a.focus == focusNotes && len(a.notes) > 0 {
			a.mode = modeConfirmClearNotes
		}
		return a, nil

	case "e":
		// Context-sensitive: on a task list it edits the row under the
		// cursor, on the notes board it expands the board.
		if a.focus == focusToday || a.focus == focusOverdue {
			return a.beginEditRow()
		}
		if a.focus == focusNotes {
			a.notesExpanded = !a.notesExpanded
			// The viewport changes size, so the scroll offset may now leave
			// the selected note off-screen.
			a.syncScroll()
		}
		return a, nil

	case "i":
		// Task-list only: neither the notes board nor Reports has items
		// with a notion of importance.
		if a.focus != focusNotes && a.focus != focusReports {
			a.toggleImportantSelected()
		}
		return a, nil

	case "I":
		a.filterImportant = !a.filterImportant
		a.clampSelections()
		return a, nil

	case "U":
		a.filterUndone = !a.filterUndone
		a.clampSelections()
		return a, nil

	case "p":
		a.pomo.running = !a.pomo.running
		return a, nil

	case "r":
		a.pomo.reset()
		a.pomo.running = false
		return a, nil

	case "n":
		a.pomo.advance()
		a.pomo.running = false
		return a, notifyPhaseChange(a.pomo.phase)

	case "t":
		name := cycleTheme()
		a.status = "Theme: " + name
		a.saveSettings()
		return a, nil

	case "L":
		a.layout = a.layout.next()
		a.status = "Layout: " + a.layout.String()
		// Selections can fall outside the new viewport: the layouts differ in
		// pane height, so a row visible in one may not exist in another.
		a.clampSelections()
		a.saveSettings()
		return a, nil
	}

	return a, nil
}

func (a App) updateAdding(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	suggestions := a.addSuggestions()

	switch msg.String() {
	case "esc":
		a.mode = modeNormal
		a.addSuggest = -1
		a.input.Blur()
		return a, nil

	case "up":
		if len(suggestions) > 0 {
			a.addSuggest--
			if a.addSuggest < -1 {
				a.addSuggest = len(suggestions) - 1
			}
			return a, nil
		}

	case "down", "tab":
		if len(suggestions) > 0 {
			a.addSuggest++
			if a.addSuggest >= len(suggestions) {
				a.addSuggest = -1
			}
			return a, nil
		}

	case "enter":
		// A highlighted suggestion attaches the row to that ticket; otherwise
		// the text stands on its own, so plain tasks stay a first-class thing
		// to add here.
		_, spec := deadline.Parse(a.input.Value(), a.now())
		if a.addSuggest >= 0 && a.addSuggest < len(suggestions) {
			a.addTicketTask(suggestions[a.addSuggest], spec)
		} else {
			a.addTask(a.input.Value())
		}
		a.mode = modeNormal
		a.addSuggest = -1
		a.input.Blur()
		return a, nil
	}

	// Any edit invalidates the highlighted row, since the list is about to be
	// recomputed from different text.
	a.addSuggest = -1

	// Bound the value to the visible field so long titles scroll horizontally
	// instead of overflowing the pane. This has to happen before Update: the
	// widget recomputes its scroll offsets from Width inside handleOverflow,
	// which View then just reads.
	a.input.Width = a.inputFieldWidth()

	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return a, cmd
}

// confirmHint is the "[y] yes / [any other key] cancel" suffix on a
// confirmation prompt. It reuses the help bar's own key/label styles so the
// prompt matches the rest of the app's key hints — the confirmation lives on
// the status line ONLY, and the help bar is left showing the normal bindings,
// so the same question isn't asked twice in two different styles.
func confirmHint() string {
	return helpLabelStyle.Render("   ") +
		helpKeyStyle.Render("[y]") + helpLabelStyle.Render(" yes") +
		helpLabelStyle.Render("   ") +
		helpKeyStyle.Render("[any other key]") + helpLabelStyle.Render(" cancel")
}

// noteEditorHeight is how many rows the inline note editor occupies. Kept
// small so the board stays visible while typing; the textarea scrolls
// internally for longer notes.
func (a App) noteEditorHeight() int {
	const h = 3
	return h
}

// renderNotesPane draws the Notes board, including the inline editor when one
// is open. Returns "" when the layout gave Notes no room, so callers can omit
// it entirely rather than render a zero-height box.
func (a App) renderNotesPane(g geometry) string {
	if g.notesHeight < 3 || g.notesWidth < 6 {
		return ""
	}
	contentWidth := g.notesWidth - 4
	if contentWidth < 1 {
		contentWidth = 1
	}

	body := renderNotes(a.notes, a.notesSelected, a.notesScroll,
		a.visibleRowsFor(focusNotes), a.focus == focusNotes,
		a.mode == modeNoteEditing, contentWidth)

	if a.mode == modeNoteEditing {
		body += "\n" + a.renderNoteEditor(contentWidth)
	}

	title := fmt.Sprintf("Notes (%d)", len(a.notes))
	return renderPane(title, body, a.focus == focusNotes, g.notesWidth, g.notesHeight)
}

// renderNoteEditor draws the multi-line note editor at the given width,
// returning exactly noteEditorHeight() lines. Shared by the Notes pane and
// simple mode.
//
// The widget's own styles go through Inline(true), which strips backgrounds,
// and it pads its lines with unstyled spaces — so its output can't be made to
// carry a background from the outside by styling alone. Strip the ANSI it
// produced and re-render each line as background-carrying spans instead. The
// editor is plain text (no per-token colors to preserve), so nothing is lost.
func (a App) renderNoteEditor(width int) string {
	bg := colorPaneBg
	if a.simple {
		// Simple mode has no panes, so the editor sits on the page surface.
		bg = colorBg
	}

	// Value receiver: these only affect the frame being rendered. Set here
	// rather than at construction so they follow theme changes.
	a.noteInput.SetWidth(width)
	a.noteInput.SetHeight(a.noteEditorHeight())
	a.noteInput.FocusedStyle.Base = lipgloss.NewStyle().Background(bg)
	a.noteInput.FocusedStyle.Text = lipgloss.NewStyle().Foreground(colorText).Background(bg)
	a.noteInput.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(colorMuted).Background(bg)
	a.noteInput.FocusedStyle.CursorLine = lipgloss.NewStyle().Background(bg)
	a.noteInput.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(colorAccent).Background(bg)

	editStyle := lipgloss.NewStyle().Foreground(colorText).Background(bg)
	promptStyle := lipgloss.NewStyle().Foreground(colorAccent).Background(bg)
	cursorStyle := lipgloss.NewStyle().Foreground(bg).Background(colorAccent)

	// Stripping the widget's ANSI also removes the cursor's inverse video, so
	// the caret is drawn from the model's own row/column instead.
	curRow, curCol := a.noteInput.Line(), a.noteInput.LineInfo().ColumnOffset

	var out []string
	for row, l := range strings.Split(a.noteInput.View(), "\n") {
		plain := strings.TrimRight(ansiRe.ReplaceAllString(l, ""), " ")
		rest, hasPrompt := strings.CutPrefix(plain, a.noteInput.Prompt)
		if !hasPrompt {
			rest = plain
		}

		var text string
		if row == curRow {
			r := []rune(rest)
			for len(r) <= curCol {
				r = append(r, ' ')
			}
			text = editStyle.Render(string(r[:curCol])) +
				cursorStyle.Render(string(r[curCol])) +
				editStyle.Render(string(r[curCol+1:]))
		} else {
			text = editStyle.Render(rest)
		}

		line := text
		if hasPrompt {
			line = promptStyle.Render(a.noteInput.Prompt) + text
		}
		if pad := width - lipgloss.Width(line); pad > 0 {
			line += lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", pad))
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// startNoteEdit opens the note editor. index -1 adds a new note, otherwise
// the existing note is loaded for editing.
func (a App) startNoteEdit(index int) (tea.Model, tea.Cmd) {
	a.mode = modeNoteEditing
	a.noteEditIndex = index
	if index >= 0 && index < len(a.notes) {
		a.noteInput.SetValue(a.notes[index].Body)
	} else {
		a.noteInput.SetValue("")
	}
	a.noteInput.Focus()
	a.noteInput.CursorEnd()
	return a, textarea.Blink
}

// updateNoteEditing handles the multi-line note editor. Enter saves;
// shift+enter and alt+enter insert a newline. Shift+enter is what the user
// asked for, but many terminals send an identical sequence for enter and
// shift+enter (they're indistinguishable without kitty-protocol or similar
// support), so alt+enter is bound alongside it as a portable fallback.
func (a App) updateNoteEditing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.mode = modeNormal
		a.noteInput.Blur()
		return a, nil

	// Newline bindings, and what each is actually worth:
	//
	//   ctrl+j    — a real control character (0x0A). Universal.
	//   ctrl+n    — mnemonic alias ("new line"). Universal, same reason.
	//   alt+enter — ESC-prefixed; this is what macOS Option+Enter sends when
	//               the terminal treats Option as Meta. Widely supported.
	//   ctrl+j    — also what Option+Enter sends when it does NOT. (Both
	//               encodings verified with a key probe.)
	//   shift+enter — bound, but only reaches the app in terminals speaking
	//               the kitty keyboard protocol (kitty, WezTerm, Ghostty,
	//               foot) or iTerm2 with a manual mapping. Legacy terminal
	//               encoding has no representation for modifier+Enter, so
	//               Terminal.app sends bytes identical to plain Enter and NO
	//               binding can separate them. Kept for terminals that can
	//               send it; deliberately not advertised as the primary
	//               route, since a hint that silently does nothing is worse
	//               than no hint.
	case "shift+enter", "alt+enter", "ctrl+j", "ctrl+n":
		a.noteInput.InsertString("\n")
		return a, nil

	case "enter":
		a.saveNote(a.noteInput.Value())
		a.mode = modeNormal
		a.noteInput.Blur()
		return a, nil
	}

	var cmd tea.Cmd
	a.noteInput, cmd = a.noteInput.Update(msg)
	return a, cmd
}

// updateConfirmClearNotes handles the whole-board clear prompt. Same
// anything-but-yes-cancels rule as the delete prompt.
func (a App) updateConfirmClearNotes(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	a.mode = modeNormal
	switch msg.String() {
	case "y", "Y", "enter":
		a.notes = nil
		a.notesSelected = 0
		a.notesScroll = 0
		a.persistNotes()
		a.status = "Notes cleared"
	default:
		a.status = "Clear cancelled"
	}
	return a, nil
}

// saveNote commits the editor's contents, either appending a new note or
// replacing the one being edited. An empty body deletes rather than storing a
// blank bullet.
func (a *App) saveNote(body string) {
	body = strings.TrimRight(body, "\n \t")
	editing := a.noteEditIndex
	a.noteEditIndex = -1

	if strings.TrimSpace(body) == "" {
		if editing >= 0 && editing < len(a.notes) {
			a.notes = append(a.notes[:editing], a.notes[editing+1:]...)
			a.clampSelections()
			a.persistNotes()
		}
		return
	}

	if editing >= 0 && editing < len(a.notes) {
		a.notes[editing].Body = body
	} else {
		a.notes = append(a.notes, model.Note{
			ID:        strconv.FormatInt(a.now().UnixNano(), 36),
			Body:      body,
			CreatedAt: a.now(),
		})
		a.notesSelected = len(a.notes) - 1
	}
	if editing >= 0 && editing < len(a.notes) {
		a.notesSelected = editing
	}
	a.persistNotes()
	// Leave the cursor on the note just written and scroll it into view, so
	// the ">" indicator lands on it rather than on wherever it was before.
	a.selectNote(a.notesSelected)
}

// selectNote puts the cursor on the given note index in whichever list is
// showing — the merged simple-mode list or the Notes board — and scrolls it
// into view.
func (a *App) selectNote(index int) {
	if index < 0 || index >= len(a.notes) {
		return
	}
	if a.simple {
		entries := a.simpleEntries()
		for i, e := range entries {
			if e.isNote && e.noteIndex == index {
				a.simpleSelected = i
				a.syncSimpleScroll(entries)
				return
			}
		}
		return
	}
	a.notesSelected = index
	a.focus = focusNotes
	a.syncScroll()
}

func (a *App) deleteSelectedNote() {
	if a.notesSelected < 0 || a.notesSelected >= len(a.notes) {
		return
	}
	a.notes = append(a.notes[:a.notesSelected], a.notes[a.notesSelected+1:]...)
	a.clampSelections()
	a.persistNotes()
}

func (a *App) persistNotes() {
	defer a.syncDailyNote()
	if a.noPersist {
		return
	}
	if err := model.SaveNotes(a.notes); err != nil {
		a.err = "failed to save notes: " + err.Error()
	}
}

// leftPaneWidth is the outer width of the Today/Overdue panes. Delegates to
// geometry() so it tracks the active layout — in the stacked layout the task
// panes span the full width rather than a 3:5 column.
func (a *App) leftPaneWidth() int { return a.geometry().taskWidth }

// inputFieldWidth is how many cells the add-task value itself may occupy:
// the pane interior (outer width less border and padding) minus the "+ "
// prompt, the widget's own one-cell prompt, and the cursor block it always
// appends past the value. Under-reserving here doesn't wrap the line, it
// pushes it a cell past the pane border, since renderPane clamps height but
// not width.
func (a *App) inputFieldWidth() int {
	w := a.leftPaneWidth() - 4 -
		lipgloss.Width(inputPromptStyle.Render("+ ")) -
		lipgloss.Width(a.input.Prompt) - 1
	if w < 1 {
		w = 1
	}
	return w
}

func (a *App) addTask(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}

	// Deadline tokens are stripped before anything else, so "!tmr" never ends
	// up in the title and never gets mistaken for the trailing clock time
	// that marks an appointment.
	raw, spec := deadline.Parse(raw, a.now())
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}

	title := raw
	taskTime := ""
	kind := model.KindTask
	fields := strings.Fields(raw)
	if len(fields) > 0 {
		last := fields[len(fields)-1]
		if isTimeLike(last) {
			taskTime = last
			kind = model.KindAppointment
			title = strings.TrimSpace(strings.TrimSuffix(raw, last))
		}
	}
	if title == "" {
		return
	}

	t := model.Task{
		ID:        strconv.FormatInt(a.now().UnixNano(), 36),
		Title:     title,
		Done:      false,
		Kind:      kind,
		Date:      a.now().Format(dateFormat),
		Time:      taskTime,
		CreatedAt: a.now(),
	}
	if spec.HasDue {
		due := spec.Due
		t.Due = &due
	}
	if spec.HasRemind {
		at := spec.RemindAt
		t.RemindAt = &at
	}

	a.tasks = append(a.tasks, t)
	a.persist()
	a.syncLocalTask(t.ID)
	a.selectTaskByID(t.ID)
}

// selectTaskByID moves the selection onto the task with the given ID and
// scrolls it into view, so a freshly added task is the one under the cursor.
// Today's list is sorted (appointments by time) and simple mode's is merged by
// creation time, so the new row is found by ID rather than assumed to be last.
// An active filter can hide the task entirely, in which case the selection is
// left where clamping puts it.
func (a *App) selectTaskByID(id string) {
	if a.simple {
		entries := a.simpleEntries()
		for i, e := range entries {
			if !e.isNote && e.task.ID == id {
				a.simpleSelected = i
				a.syncSimpleScroll(entries)
				return
			}
		}
		return
	}
	for i, t := range a.todayTasks() {
		if t.ID == id {
			a.todaySelected = i
			// Selection only follows the cursor in the pane that owns it;
			// adding is Today-only, so focus Today to make the move visible.
			a.focus = focusToday
			a.syncScroll()
			return
		}
	}
	a.clampSelections()
}

func isTimeLike(s string) bool {
	if len(s) != 5 || s[2] != ':' {
		return false
	}
	h, err1 := strconv.Atoi(s[0:2])
	m, err2 := strconv.Atoi(s[3:5])
	if err1 != nil || err2 != nil {
		return false
	}
	return h >= 0 && h < 24 && m >= 0 && m < 60
}

func (a *App) moveSelection(delta int) {
	n := len(a.currentList())
	switch a.focus {
	case focusNotes:
		n = len(a.notes)
	case focusToday:
		// Today addresses rows, not tasks: an expanded ticket contributes a
		// line per subtask, so a task count would stop short of the list.
		n = len(a.todayRows())
	}
	if n == 0 {
		return
	}
	sel := a.currentSelected()
	sel += delta
	if sel < 0 {
		sel = 0
	}
	if sel >= n {
		sel = n - 1
	}
	a.setCurrentSelected(sel)
	a.syncScroll()
}

// syncScroll keeps the current pane's scroll offset such that the selected
// row stays within the visible viewport, scrolling the minimal amount needed.
func (a *App) syncScroll() {
	visible := a.visibleRowsFor(a.focus)
	scroll := a.currentScroll()

	// On the notes board the selection is a NOTE index while the scroll
	// offset is a DISPLAY-LINE offset (a note wraps to several lines), so the
	// selected note's line span has to be resolved before they can be
	// compared — treating the two as the same unit scrolls to the wrong row
	// as soon as any note wraps.
	if a.focus == focusNotes {
		first, last := a.noteLineSpan(a.notesSelected)
		if first < 0 {
			return
		}
		if first < scroll {
			scroll = first
		} else if last >= scroll+visible {
			// Prefer showing the note's start when it's taller than the pane,
			// rather than its end.
			scroll = last - visible + 1
			if scroll > first {
				scroll = first
			}
		}
		if scroll < 0 {
			scroll = 0
		}
		a.notesScroll = scroll
		return
	}

	sel := a.currentSelected()
	if sel < scroll {
		scroll = sel
	} else if sel >= scroll+visible {
		scroll = sel - visible + 1
	}
	if scroll < 0 {
		scroll = 0
	}
	a.setCurrentScroll(scroll)
}

// noteLineSpan returns the first and last display-line indexes occupied by
// the given note at the current pane width, or (-1, -1) if out of range.
func (a App) noteLineSpan(index int) (int, int) {
	if index < 0 || index >= len(a.notes) {
		return -1, -1
	}
	lines := noteLines(a.notes, a.notesContentWidth())
	first, last := -1, -1
	for i, l := range lines {
		if l.noteIndex == index {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	return first, last
}

// notesContentWidth is the interior text width of the Notes pane.
func (a App) notesContentWidth() int {
	w := a.geometry().notesWidth - 4
	if w < 1 {
		w = 1
	}
	return w
}

func (a *App) clampSelections() {
	todayLen := len(a.todayRows())
	if a.todaySelected >= todayLen {
		a.todaySelected = todayLen - 1
	}
	if a.todaySelected < 0 {
		a.todaySelected = 0
	}
	overdueLen := len(a.overdueTasks())
	if a.overdueSelected >= overdueLen {
		a.overdueSelected = overdueLen - 1
	}
	if a.overdueSelected < 0 {
		a.overdueSelected = 0
	}
	if a.notesSelected >= len(a.notes) {
		a.notesSelected = len(a.notes) - 1
	}
	if a.notesSelected < 0 {
		a.notesSelected = 0
	}
	a.syncScroll()
}

// actionTaskID is the task a keystroke applies to in the focused pane.
//
// Today addresses rows and the other panes address tasks, so this is the one
// place that difference is resolved; acting on a raw index would hit the wrong
// task whenever a ticket is expanded.
func (a App) actionTaskID() string {
	if a.focus == focusToday {
		return a.selectedTodayTaskID()
	}
	list := a.currentList()
	sel := a.currentSelected()
	if sel < 0 || sel >= len(list) {
		return ""
	}
	return list[sel].ID
}

func (a *App) toggleSelected() {
	id := a.actionTaskID()
	if id == "" {
		return
	}
	for i := range a.tasks {
		if a.tasks[i].ID == id {
			a.tasks[i].Done = !a.tasks[i].Done
			if a.tasks[i].Done {
				now := a.now()
				a.tasks[i].DoneAt = &now
			} else {
				a.tasks[i].DoneAt = nil
			}
			break
		}
	}
	a.persist()
}

func (a *App) toggleImportantSelected() {
	id := a.actionTaskID()
	if id == "" {
		return
	}
	for i := range a.tasks {
		if a.tasks[i].ID == id {
			a.tasks[i].Important = !a.tasks[i].Important
			break
		}
	}
	a.persist()
}

// saveSettings persists every user preference at once. Settings are written
// as a whole struct, so saving one field from a freshly-built Settings{} would
// blank the others — always send the full current state.
func (a App) saveSettings() {
	if a.noPersist {
		return
	}
	_ = model.SaveSettings(a.settings())
}

// selectedTask returns the task under the cursor in the focused pane, or nil
// when the pane is empty (or the selection is somehow out of range).
func (a *App) selectedTask() *model.Task {
	list := a.currentList()
	sel := a.currentSelected()
	if sel < 0 || sel >= len(list) {
		return nil
	}
	return &list[sel]
}

// updateConfirmDelete handles the y/n prompt shown before a delete. Anything
// other than an explicit confirmation cancels, so a stray keypress can't
// destroy a task.
func (a App) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		switch {
		case a.simple:
			a.deleteSimpleSelected()
		case a.focus == focusNotes:
			a.deleteSelectedNote()
		default:
			a.deleteSelected()
		}
		a.mode = modeNormal
		return a, nil
	default:
		a.mode = modeNormal
		a.status = "Delete cancelled"
		return a, nil
	}
}

func (a *App) deleteSelected() {
	list := a.currentList()
	sel := a.currentSelected()
	if sel < 0 || sel >= len(list) {
		return
	}
	id := list[sel].ID
	filtered := a.tasks[:0]
	for _, t := range a.tasks {
		if t.ID != id {
			filtered = append(filtered, t)
		}
	}
	a.tasks = filtered

	newLen := len(a.currentList())
	if sel >= newLen {
		sel = newLen - 1
	}
	a.setCurrentSelected(sel)
	a.syncScroll()
	a.persist()
}

func (a *App) persist() {
	if a.noPersist {
		return
	}
	if err := model.Save(a.tasks); err != nil {
		a.err = "failed to save tasks: " + err.Error()
	}
}

// currentList is the task list of the focused pane. With Notes focused there
// is no task list, so it returns nil rather than falling through to Overdue —
// paired with currentSelected() returning notesSelected, an `else` here made
// every task operation silently act on an arbitrary overdue row.
func (a App) currentList() []model.Task {
	switch a.focus {
	case focusToday:
		return a.todayTasks()
	case focusOverdue:
		return a.overdueTasks()
	default:
		return nil
	}
}

func (a App) currentSelected() int {
	switch a.focus {
	case focusToday:
		return a.todaySelected
	case focusNotes:
		return a.notesSelected
	case focusOverdue:
		return a.overdueSelected
	default:
		// focusReports has no list; nothing meaningful to select.
		return 0
	}
}

func (a *App) setCurrentSelected(v int) {
	switch a.focus {
	case focusToday:
		a.todaySelected = v
	case focusNotes:
		a.notesSelected = v
	case focusOverdue:
		a.overdueSelected = v
	}
}

func (a App) currentScroll() int {
	switch a.focus {
	case focusToday:
		return a.todayScroll
	case focusNotes:
		return a.notesScroll
	case focusOverdue:
		return a.overdueScroll
	default:
		return 0
	}
}

func (a *App) setCurrentScroll(v int) {
	switch a.focus {
	case focusToday:
		a.todayScroll = v
	case focusNotes:
		a.notesScroll = v
	case focusOverdue:
		a.overdueScroll = v
	}
}

func (a App) applyFilters(tasks []model.Task) []model.Task {
	if !a.filterImportant && !a.filterUndone {
		return tasks
	}
	out := make([]model.Task, 0, len(tasks))
	for _, t := range tasks {
		if a.filterImportant && !t.Important {
			continue
		}
		if a.filterUndone && t.Done {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (a App) todayTasks() []model.Task {
	today := a.now().Format(dateFormat)
	var out []model.Task
	for _, t := range a.tasks {
		if t.Date == today {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Time == out[j].Time {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		if out[i].Time == "" {
			return false
		}
		if out[j].Time == "" {
			return true
		}
		return out[i].Time < out[j].Time
	})
	return a.applyFilters(out)
}

// overdueTasks collects both kinds of late work: a task carried over from an
// earlier day, and a task whose deadline has passed regardless of which day it
// is filed under. Missed deadlines sort first — they are the stronger claim on
// attention, and a task due today can be late while still belonging to today.
func (a App) overdueTasks() []model.Task {
	now := a.now()
	today := now.Format(dateFormat)
	var out []model.Task
	for _, t := range a.tasks {
		if t.Done {
			continue
		}
		if t.Date < today || t.PastDue(now) {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		li, lj := out[i].PastDue(now), out[j].PastDue(now)
		if li != lj {
			return li
		}
		if li && lj && !out[i].Deadline().Equal(out[j].Deadline()) {
			return out[i].Deadline().Before(out[j].Deadline())
		}
		return out[i].Date < out[j].Date
	})
	return a.applyFilters(out)
}

// jiraHelpGroup hides Jira keys when the integration is off, rather than
// advertising bindings that would only report that it is disabled.
func jiraHelpGroup(enabled bool) helpGroup {
	if !enabled {
		return helpGroup{"Time", []helpKey{{"w", "log time"}}}
	}
	return helpGroup{"Jira", []helpKey{{"R", "fetch"}, {"s", "status"}, {"c", "comment"}, {"w", "log time"}}}
}

func (a App) helpGroups() []helpGroup {
	if a.simple && a.mode == modeNormal {
		what := "task"
		if a.simpleNoteMode {
			what = "note"
		}
		return []helpGroup{
			{"", []helpKey{
				{"a", "add " + what}, {"tab", "switch to " + map[bool]string{true: "task", false: "note"}[a.simpleNoteMode]},
				{"space/enter", "toggle/edit"}, {"d", "delete"}, {"i", "important"},
				{"↑/↓ j/k", "navigate"}, {"t", "theme"}, {"q", "quit"},
			}},
		}
	}

	if a.settingsUI.open {
		if a.settingsUI.editing {
			return []helpGroup{{"", []helpKey{{"enter", "save"}, {"esc", "cancel"}}}}
		}
		return []helpGroup{{"", []helpKey{
			{"↑/↓ j/k", "field"}, {"enter", "edit/cycle"}, {"esc", "close"},
		}}}
	}

	switch a.view {
	case viewPARA:
		if a.mode == modeVaultAdding || a.mode == modeTicketNote {
			return []helpGroup{{"", []helpKey{{"enter", "save"}, {"esc", "cancel"}}}}
		}
		return []helpGroup{
			{"View", []helpKey{{"1/2/3", "views"}, {",", "settings"}, {"?", "all keys"}}},
			{"Move", []helpKey{{"tab", "pane"}, {"↑/↓ j/k", "select"}, {"enter", "toggle"}}},
			{"Vault", []helpKey{{"a", "add task"}, {"n", "add note"}, {"u", "set area"}, {"o", "open"}, {"r", "reindex"}}},
			jiraHelpGroup(a.jiraEnabled),
			{"", []helpKey{{"p", "track time"}, {"C", "export .ics"}, {"q", "quit"}}},
		}
	case viewCalendar:
		return []helpGroup{
			{"View", []helpKey{{"1/2/3", "views"}, {",", "settings"}, {"?", "all keys"}}},
			{"", []helpKey{{"r", "reindex"}, {"C", "export .ics"}, {"q", "quit"}}},
		}
	}

	// While a confirmation is up the help bar is emptied rather than
	// duplicated: the prompt on the status line already carries its own
	// [y]/[any other key] hint, and every other binding is inert until the
	// prompt is answered, so listing them would offer keys that do nothing.
	if a.mode == modeConfirmDelete || a.mode == modeConfirmClearNotes {
		return nil
	}
	if a.mode == modeNoteEditing {
		return []helpGroup{
			{"", []helpKey{
				{"enter", "save"}, {"ctrl+j / opt+enter", "new line"}, {"esc", "cancel"},
			}},
		}
	}
	if a.mode == modeAdding {
		return []helpGroup{
			{"", []helpKey{
				{"enter", "confirm"}, {"esc", "cancel"},
				{"", "end with HH:MM to add it as an appointment"},
			}},
		}
	}
	if a.focus == focusNotes {
		notesKeys := []helpKey{
			{"a", "add"}, {"enter", "edit"}, {"d", "delete"}, {"C", "clear board"},
		}
		viewKeys := []helpKey{{"tab", "switch pane"}, {"↑/↓ j/k", "navigate"}}
		if a.notesExpanded {
			notesKeys = append(notesKeys, helpKey{"e", "shrink"})
			// No pane to switch to while expanded, and tab is inert there.
			viewKeys = []helpKey{{"↑/↓ j/k", "navigate"}}
		} else {
			notesKeys = append(notesKeys, helpKey{"e", "expand"})
		}
		return []helpGroup{
			{"Notes", notesKeys},
			{"View", viewKeys},
			{"App", []helpKey{
				{"t", "theme"}, {"L", "layout"}, {"q", "quit"},
			}},
		}
	}
	if a.focus == focusReports {
		// The Reports pane has no list and no items, so none of the task or
		// note bindings apply — only chart navigation and the app-wide keys.
		return []helpGroup{
			{"Chart", []helpKey{
				{"←/→ h/l", "switch chart"}, {"", a.reportChart.String()},
			}},
			{"View", []helpKey{{"tab", "switch pane"}}},
			{"App", []helpKey{
				{"t", "theme"}, {"L", "layout"}, {"q", "quit"},
			}},
		}
	}

	// Adding is Today-only (there's no such thing as adding a task that's
	// already overdue), so the hint is omitted when Overdue has focus rather
	// than advertising a key that does nothing.
	taskKeys := []helpKey{{"a", "add"}}
	if a.focus == focusOverdue {
		taskKeys = nil
	}
	taskKeys = append(taskKeys,
		helpKey{"e", "edit"}, helpKey{"space/enter", "toggle"},
		helpKey{"D", "deadline"}, helpKey{"d", "delete"}, helpKey{"i", "important"})
	if a.focus == focusToday {
		taskKeys = append(taskKeys, helpKey{"A", "subtask"}, helpKey{"z", "fold"})
	}

	return []helpGroup{
		{"Task", taskKeys},
		{"View", []helpKey{
			{"tab", "switch pane"}, {"↑/↓ j/k", "navigate"}, {"I/U", "filters"},
			{"1/2/3", "views"}, {"?", "all keys"},
		}},
		// Pomodoro's keys aren't listed here — they're rendered inside the
		// Pomodoro pane itself, next to the thing they control.
		{"App", []helpKey{
			{"t", "theme"}, {"L", "layout"}, {"q", "quit"},
		}},
	}
}

// chromeLines returns how many lines of fixed UI (error line, help bar)
// surround the body panes, so both visibleRowsFor and View agree on how much
// height the panes actually get. The top header/date bar was removed (the
// date now lives in the greeting pane, which is part of the body, not fixed
// chrome). The help bar's line count varies with terminal width (it wraps to
// a second line when narrow), so a flat constant here would let the last
// help line get clipped off-screen or mis-budget the body panes' height by
// one row whenever wrapping kicks in.
func (a App) chromeLines() int {
	const errLine = 1
	// Measured against the NORMAL bindings, not whatever the current mode
	// shows. A confirmation empties the help bar, and a mode-sensitive
	// measurement would then hand those rows to the panes — so opening a
	// prompt made every pane grow, and answering it made them shrink back,
	// jumping the layout underneath the question being asked.
	normal := a
	normal.mode = modeNormal
	help := lipgloss.Height(renderHelpBar(normal.helpGroups(), a.width))
	return errLine + help
}

// visibleRowsFor computes how many task rows fit in the given pane's content
// height. Mirrors the height math used when actually laying out the panes in
// View(), so scroll math and rendering never disagree about viewport size.
func (a App) visibleRowsFor(focus focusedPane) int {
	g := a.geometry()
	paneHeight := g.todayHeight
	switch focus {
	case focusOverdue:
		paneHeight = g.overdueHeight
	case focusNotes:
		paneHeight = g.notesHeight
	}
	contentHeight := paneHeight - 2 // border top+bottom
	// No title row is reserved: the pane title is drawn ON the top border by
	// renderPane, so it costs no body line.
	//
	// renderTaskList always emits a scroll-indicator line (blank when there's
	// nothing to scroll), so its line is reserved unconditionally here —
	// otherwise the pane grows by a line whenever "N more" starts appearing.
	indicatorLines := 1
	rows := contentHeight - indicatorLines
	if focus == focusToday && (a.mode == modeAdding || a.mode == modeEditRow || a.mode == modeAddSubtask) {
		rows-- // reserve a line for the inline input
		// The typeahead sits under the input, so its rows come out of the
		// list too or the pane would grow as you type.
		rows -= len(a.addSuggestions())
	}
	if focus == focusNotes && a.mode == modeNoteEditing {
		// The note editor is multi-line, so reserve its full height.
		rows -= a.noteEditorHeight()
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func filterLabel(important, undone bool) string {
	var tags []string
	if important {
		tags = append(tags, "Important")
	}
	if undone {
		tags = append(tags, "Undone")
	}
	if len(tags) == 0 {
		return ""
	}
	return " [" + strings.Join(tags, ", ") + "]"
}

func (a App) View() string {
	if a.width == 0 {
		return "loading..."
	}

	helpLine := renderHelpBar(a.helpGroups(), a.width)

	// chromeLines reserves the help bar's NORMAL height so the panes don't
	// resize when a mode blanks it (see there). Pad back up to that height
	// with empty rows, or the page would render short of the terminal and
	// leave unpainted lines at the bottom.
	if want := a.chromeLines() - 1; lipgloss.Height(helpLine) < want {
		helpLine += strings.Repeat("\n", want-lipgloss.Height(helpLine))
	}

	if a.simple {
		return a.assemblePage(a.renderSimple(), helpLine)
	}

	// Settings replace the body rather than floating over it: a true overlay
	// would have to composite against whichever view is behind it, and every
	// pane here is already a fixed-size block.
	if a.settingsUI.open {
		return a.assemblePage(a.renderSettings(), helpLine)
	}
	if a.picker.open {
		return a.assemblePage(a.renderDeadlinePicker(), helpLine)
	}
	if a.showKeys {
		return a.assemblePage(a.renderKeyHelp(), helpLine)
	}

	switch a.view {
	case viewPARA:
		return a.assemblePage(a.renderPara(), helpLine)
	case viewCalendar:
		return a.assemblePage(a.renderCalendar(), helpLine)
	}

	g := a.geometry()

	// Expanded Notes replaces the whole body — no other pane is built, since
	// geometry gave them all zero height.
	if a.notesExpanded && a.focus == focusNotes {
		return a.assemblePage(a.renderNotesPane(g), helpLine)
	}

	leftWidth := g.taskWidth
	rightWidth := g.infoWidth
	todayHeight := g.todayHeight
	overdueHeight := g.overdueHeight

	filters := filterLabel(a.filterImportant, a.filterUndone)

	today := a.todayTasks()
	todayVisible := a.visibleRowsFor(focusToday)
	todayRows := a.todayRows()
	todayBody := a.renderTodayRows(todayRows, a.todaySelected, a.todayScroll, todayVisible, a.focus == focusToday, leftWidth-4)
	if a.mode == modeAdding || a.mode == modeEditRow || a.mode == modeAddSubtask {
		// Set here rather than once in NewApp so these follow theme changes.
		a.input.TextStyle = lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)
		a.input.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
		a.input.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent).Background(colorPaneBg)
		a.input.Cursor.Style = lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)

		// Clip the placeholder to the field. The widget truncates a typed
		// *value* to Width but never its placeholder, so on a narrow pane the
		// full hint text ran past the border — and then vanished to a correct
		// width on the first keystroke, reading as the line resizing as soon
		// as you started typing.
		if field := a.inputFieldWidth(); lipgloss.Width(a.input.Placeholder) > field {
			a.input.Placeholder = fitToWidth(a.input.Placeholder, field)
		}

		// Render with Width unset and do the trailing fill ourselves. The
		// widget's own padding differs between its two branches — the
		// placeholder path pads to Width while the typed path pads to Width
		// and *then* appends a cursor cell past it — so letting it size the
		// line made the row jump wider the moment a key was pressed. Its
		// padding also goes through TextStyle, emerging wrapped in SGR
		// codes that a TrimRight(" ") can't strip back off.
		//
		// Width still matters for horizontal scrolling of long values, but
		// that's consumed in Update (handleOverflow), not here, so clearing
		// it at render time costs nothing. View has a value receiver, so
		// this only touches the local copy used for this frame.
		a.input.Width = 0
		inputLine := inputPromptStyle.Render("+ ") + a.input.View()
		if pad := (leftWidth - 4) - lipgloss.Width(inputLine); pad > 0 {
			inputLine += lipgloss.NewStyle().Background(colorPaneBg).Render(strings.Repeat(" ", pad))
		}
		todayBody += "\n" + inputLine
		if s := a.renderSuggestions(a.addSuggestions(), leftWidth-4); s != "" {
			todayBody += "\n" + s
		}
	}
	todayPane := renderPane(fmt.Sprintf("Today (%d)%s", len(today), filters), todayBody, a.focus == focusToday, leftWidth, todayHeight)

	overdue := a.overdueTasks()
	// In the stacked layout Today, Overdue and Notes share the row equally
	// (geometry gives Notes the rounding remainder); in the column layouts
	// Today and Overdue are stacked at the same width.
	overdueWidth := leftWidth
	overdueVisible := a.visibleRowsFor(focusOverdue)
	overdueBody := renderTaskList(decorateDeadlines(overdue, a.now()), a.overdueSelected, a.overdueScroll, overdueVisible, a.focus == focusOverdue, true, overdueWidth-4)
	overduePane := renderPane(fmt.Sprintf("Overdue (%d)%s", len(overdue), filters), overdueBody, a.focus == focusOverdue, overdueWidth, overdueHeight)

	tasks := lipgloss.JoinVertical(lipgloss.Left, todayPane, overduePane)
	if a.layout == layoutStacked {
		tasks = lipgloss.JoinHorizontal(lipgloss.Top, todayPane, overduePane)
	}

	// In the stacked layout the three info panes sit side by side, so the
	// last one absorbs the width remainder from the /3 split; in the column
	// layouts they're all the same width and the remainder is zero.
	greetWidth, reportsWidth, pomoWidth := rightWidth, rightWidth, rightWidth
	if a.layout == layoutStacked {
		pomoWidth = a.width - greetWidth - reportsWidth
	}

	greetBody := renderGreeting(a.now(), a.username, greetWidth-4, g.greetHeight-2)
	greetPane := renderPane("", greetBody, false, greetWidth, g.greetHeight)

	report := stats.Compute(a.tasks, a.now())
	reportsBody := renderReports(report, reportsWidth-4, g.reportsHeight-2, a.reportChart, a.focus == focusReports)
	reportsPane := renderPane("Reports", reportsBody, a.focus == focusReports, reportsWidth, g.reportsHeight)

	pomoBody := renderPomodoro(a.pomo, pomoWidth-4, g.pomoHeight-2)
	pomoPane := renderPane("Pomodoro", pomoBody, false, pomoWidth, g.pomoHeight)

	// The timeline sits directly above Notes, in the height taken from it.
	timelinePane := ""
	if g.timelineHeight > 0 {
		timelinePane = a.renderTimelinePane(g.notesWidth, g.timelineHeight)
	}

	notesPane := a.renderNotesPane(g)

	// The timeline occupies the height taken from Notes, so the two travel
	// together: wherever the board goes in a layout, the timeline sits
	// directly above it.
	notesStack := notesPane
	switch {
	case timelinePane != "" && notesPane != "":
		notesStack = lipgloss.JoinVertical(lipgloss.Left, timelinePane, notesPane)
	case timelinePane != "":
		notesStack = timelinePane
	}

	// In the stacked layout Notes is a third task-row column; in the
	// three-column layout it's a column of its own; otherwise it's the last
	// pane of the info column.
	if a.layout == layoutStacked && notesStack != "" {
		tasks = lipgloss.JoinHorizontal(lipgloss.Top, tasks, notesStack)
	}

	infoPanes := []string{greetPane, reportsPane, pomoPane}
	if a.layout != layoutStacked && a.layout != layoutThreeColumn && notesStack != "" {
		infoPanes = append(infoPanes, notesStack)
	}

	// The gutter between columns is a styled space, not a bare one: an
	// unstyled space here would be a column of terminal-default background
	// running the full height of the page.
	gutter := gutterColumn(lipgloss.Height(tasks))

	var body string
	switch a.layout {
	case layoutTasksRight:
		info := lipgloss.JoinVertical(lipgloss.Left, infoPanes...)
		body = lipgloss.JoinHorizontal(lipgloss.Top, info, gutter, tasks)
	case layoutStacked:
		info := lipgloss.JoinHorizontal(lipgloss.Top, greetPane, reportsPane, pomoPane)
		body = lipgloss.JoinVertical(lipgloss.Left, info, tasks)
	case layoutThreeColumn:
		info := lipgloss.JoinVertical(lipgloss.Left, infoPanes...)
		body = lipgloss.JoinHorizontal(lipgloss.Top, info, gutter, tasks, gutter, notesStack)
	default:
		info := lipgloss.JoinVertical(lipgloss.Left, infoPanes...)
		body = lipgloss.JoinHorizontal(lipgloss.Top, tasks, gutter, info)
	}

	return a.assemblePage(body, helpLine)
}

// assemblePage stacks the body, the status/prompt line and the help bar into
// the final frame, padding every line to the terminal width. Shared by the
// normal layouts and the expanded Notes view so the status line, prompts and
// background padding behave identically in both.
func (a App) assemblePage(body, helpLine string) string {
	pageBg := lipgloss.NewStyle().Background(colorBg)

	errLine := ""
	switch {
	case a.mode == modeConfirmClearNotes:
		errLine = lipgloss.NewStyle().Bold(true).Foreground(colorDanger).Background(colorBg).
			Render(fmt.Sprintf("Clear all %d notes?", len(a.notes))) +
			confirmHint()

	case a.mode == modeConfirmDelete:
		// The confirmation takes over the status line so it's impossible to
		// miss, and names the item so there's no doubt about what's going.
		prompt := "Delete this task?"
		if a.simple {
			entries := a.simpleEntries()
			if a.simpleSelected >= 0 && a.simpleSelected < len(entries) {
				e := entries[a.simpleSelected]
				if e.isNote {
					prompt = fmt.Sprintf("Delete %q?", strings.SplitN(e.note.Body, "\n", 2)[0])
				} else {
					prompt = fmt.Sprintf("Delete %q?", e.task.Title)
				}
			}
		} else if a.focus == focusNotes {
			prompt = "Delete this note?"
			if a.notesSelected >= 0 && a.notesSelected < len(a.notes) {
				// First line only: a note can be many lines long.
				first := strings.SplitN(a.notes[a.notesSelected].Body, "\n", 2)[0]
				prompt = fmt.Sprintf("Delete %q?", first)
			}
		} else if t := a.selectedTask(); t != nil {
			prompt = fmt.Sprintf("Delete %q?", t.Title)
		}
		errLine = lipgloss.NewStyle().Bold(true).Foreground(colorDanger).Background(colorBg).Render(prompt) +
			confirmHint()
	case a.err != "":
		errLine = lipgloss.NewStyle().Foreground(colorDanger).Background(colorBg).Render(a.err)
	case a.status != "":
		errLine = lipgloss.NewStyle().Foreground(colorMuted).Background(colorBg).Render(a.status)
	}

	// Every line from every source below is padded to a.width with its own
	// styled (colorBg) trailing spaces BEFORE joining — not after. Two
	// distinct bugs made that necessary: (1) any line already carrying its
	// own ANSI styling can't have background backfilled past its own
	// embedded reset by a later outer Render() call (documented repeatedly
	// elsewhere in this codebase); and (2) lipgloss.JoinVertical pads
	// shorter lines up to the widest line's width using its OWN plain,
	// unstyled spaces — so even a line that already had its trailing
	// padding correctly backgrounded gets MORE, unstyled padding appended
	// on top by JoinVertical itself if a sibling line (here, body) is
	// wider. Pre-padding every line to the same final width so none of
	// them differ removes JoinVertical's padding from the equation
	// entirely — it never has anything left to pad.

	padLines := func(s string) string {
		lines := strings.Split(s, "\n")
		for i, l := range lines {
			switch pad := a.width - lipgloss.Width(l); {
			case pad > 0:
				lines[i] = l + pageBg.Render(strings.Repeat(" ", pad))
			case pad < 0:
				// Clamp too: an overlong line (e.g. the delete prompt naming
				// a very long task title) would otherwise push the page past
				// the terminal edge, since padding alone only ever grows.
				lines[i] = truncateANSI(l, a.width)
			}
		}
		return strings.Join(lines, "\n")
	}

	// In simple mode the body carries its own side margins, so only the
	// status and help lines are indented here — otherwise they'd sit flush
	// against the terminal edge while everything above is held off it (and
	// indenting the body too would apply its margin twice). `own` is the
	// margin the line already has (renderHelpBar prefixes one space), which
	// is subtracted so everything lands on the same column.
	indentLines := func(s string, own int) string {
		if !a.simple || s == "" {
			return s
		}
		n := simplePadding - own
		if n <= 0 {
			return s
		}
		pre := pageBg.Render(strings.Repeat(" ", n))
		lines := strings.Split(s, "\n")
		for i, l := range lines {
			if strings.TrimSpace(ansiRe.ReplaceAllString(l, "")) != "" {
				lines[i] = pre + l
			}
		}
		return strings.Join(lines, "\n")
	}

	full := lipgloss.JoinVertical(lipgloss.Left,
		padLines(body),
		padLines(indentLines(errLine, 0)),
		padLines(indentLines(helpLine, 1)),
	)
	return full
}
