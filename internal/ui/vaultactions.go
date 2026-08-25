package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"taskii/internal/obsidian"
	"taskii/internal/vault"
	"taskii/internal/worklog"
)

// quickAddHeading is the section a quick-added task lands in. It is the
// freeform part of the vault's ticket template — the synced sections are
// rewritten wholesale by the Jira plugin on every fetch, so writing there
// would lose the task at the next sync.
const quickAddHeading = "Notes & Sub-tasks"

// worklogHeading is the freeform section tracked time is appended to.
const worklogHeading = "Work Log / Updates"

// resolveICSPath decides where the calendar is written.
func (a App) resolveICSPath() string {
	if a.icsSetting != "" {
		return a.icsSetting
	}
	if a.vaultPath == "" {
		return ""
	}
	return filepath.Join(a.vaultPath, "Calendar", "taskii.ics")
}

// pendingTimeFor is tracked-but-unlogged time for a key, "" when there is none.
func (a App) pendingTimeFor(key string) string {
	if a.wl == nil || key == "" {
		return ""
	}
	return worklog.Format(a.wl.Get(key).Pending())
}

// updatePara is the PARA view's key map.
func (a App) updatePara(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		a.vault.pane = (a.vault.pane + 1) % paraPaneCount
		return a, nil
	case "shift+tab":
		a.vault.pane = (a.vault.pane + paraPaneCount - 1) % paraPaneCount
		return a, nil

	case "up", "k":
		a.moveParaSelection(-1)
		return a, nil
	case "down", "j":
		a.moveParaSelection(1)
		return a, nil

	case " ", "enter":
		if a.vault.pane == paraDetail {
			return a.toggleSelectedCheckbox()
		}
		a.vault.pane = (a.vault.pane + 1) % paraPaneCount
		return a, nil

	case "a":
		return a.beginQuickAdd()

	case "n":
		return a.beginTicketNote()

	case "u":
		return a.cycleAreaOnSelected()

	case "o":
		return a.openSelectedInObsidian()

	case "r":
		return a, loadIndex(a.vaultPath, a.loc)

	case "C":
		return a, exportICS(a.vaultPath, a.icsOut, a.tasks, a.loc)

	case "R":
		return a.jiraAction("fetch from Jira", func(c *obsidian.Client, ctx context.Context, _ string) (string, error) {
			return c.FetchAll(ctx)
		})

	case "s":
		return a.jiraAction("status transition", (*obsidian.Client).TransitionStatus)

	case "c":
		return a.jiraAction("add comment", (*obsidian.Client).AddComment)

	case "w":
		return a.flushWorklog()

	case "p":
		// Binding the timer to a ticket is what makes tracked time
		// attributable; an unbound pomodoro credits nothing.
		if t, ok := a.selectedTicket(); ok {
			a.pomoKey = t.Key
			a.pomoCounted = 0
		}
		a.pomo.toggle()
		a.status = "Pomodoro tracking " + a.pomoKey
		return a, nil
	}
	return a, nil
}

// moveParaSelection moves within whichever pane has focus, resetting the panes
// to its right — a ticket selection means nothing once the area changes.
func (a *App) moveParaSelection(delta int) {
	switch a.vault.pane {
	case paraTree:
		rows := a.treeRows()
		if len(rows) == 0 {
			return
		}
		a.vault.treeSel = clamp(a.vault.treeSel+delta, 0, len(rows)-1)
		a.vault.treeScroll = scrollWindow(a.vault.treeSel, a.vault.treeScroll, a.paraGeometry().height-2, len(rows))
		a.vault.listSel, a.vault.listScroll = 0, 0
		a.vault.detailSel, a.vault.detailScroll = 0, 0
	case paraList:
		list := a.visibleTickets()
		if len(list) == 0 {
			return
		}
		a.vault.listSel = clamp(a.vault.listSel+delta, 0, len(list)-1)
		a.vault.listScroll = scrollWindow(a.vault.listSel, a.vault.listScroll, a.paraGeometry().height-2, len(list))
		a.vault.detailSel, a.vault.detailScroll = 0, 0
	case paraDetail:
		t, ok := a.selectedTicket()
		if !ok || len(t.Checkboxes) == 0 {
			return
		}
		a.vault.detailSel = clamp(a.vault.detailSel+delta, 0, len(t.Checkboxes)-1)
	}
}

// toggleSelectedCheckbox flips one task line in the note.
func (a App) toggleSelectedCheckbox() (tea.Model, tea.Cmd) {
	t, ok := a.selectedTicket()
	if !ok {
		return a, nil
	}
	c, ok := a.selectedCheckbox()
	if !ok {
		return a, nil
	}
	// The text is passed so the write refuses if the file moved on since the
	// index was built.
	if err := vault.SetCheckbox(t.Path, c.Line, c.Text, !c.Done); err != nil {
		a.err = err.Error()
		// A stale write means our picture of the note is wrong, so reindex.
		return a, loadIndex(a.vaultPath, a.loc)
	}
	a.status = "toggled: " + c.Text
	return a, loadIndex(a.vaultPath, a.loc)
}

// beginQuickAdd opens the input for a new task line.
func (a App) beginQuickAdd() (tea.Model, tea.Cmd) {
	if _, ok := a.selectedTicket(); !ok {
		a.err = "select a ticket first"
		return a, nil
	}
	a.mode = modeVaultAdding
	a.input.SetValue("")
	a.input.Placeholder = "task to add under " + quickAddHeading
	a.input.Focus()
	return a, nil
}

// commitQuickAdd writes the task into the note.
func (a App) commitQuickAdd(text string) (tea.Model, tea.Cmd) {
	text = strings.TrimSpace(text)
	a.mode = modeNormal
	a.input.Blur()
	if text == "" {
		return a, nil
	}
	t, ok := a.selectedTicket()
	if !ok {
		return a, nil
	}
	if err := vault.AppendCheckbox(t.Path, quickAddHeading, text); err != nil {
		a.err = err.Error()
		return a, nil
	}
	a.status = "added to " + t.Key
	return a, loadIndex(a.vaultPath, a.loc)
}

// beginTicketNote opens the input for a note against the selected ticket.
func (a App) beginTicketNote() (tea.Model, tea.Cmd) {
	if _, ok := a.selectedTicket(); !ok {
		a.err = "select a ticket first"
		return a, nil
	}
	a.mode = modeTicketNote
	a.input.SetValue("")
	a.input.Placeholder = "note for this ticket"
	a.input.Focus()
	return a, nil
}

// updateTicketNote handles the note input.
func (a App) updateTicketNote(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.mode = modeNormal
		a.input.Blur()
		a.input.SetValue("")
		return a, nil
	case "enter":
		return a.commitTicketNote(a.input.Value())
	}
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return a, cmd
}

// commitTicketNote appends a dated note to the ticket.
//
// It goes to the freeform work-log section, which jira-sync leaves alone.
// The synced sections are rewritten wholesale on every fetch, so a note
// written there would disappear at the next sync.
func (a App) commitTicketNote(text string) (tea.Model, tea.Cmd) {
	text = strings.TrimSpace(text)
	a.mode = modeNormal
	a.input.Blur()
	a.input.SetValue("")
	if text == "" {
		return a, nil
	}
	t, ok := a.selectedTicket()
	if !ok {
		return a, nil
	}
	line := fmt.Sprintf("- %s — %s", time.Now().In(a.loc).Format("2006-01-02 15:04"), text)
	if err := vault.AppendUnderHeading(t.Path, worklogHeading, line); err != nil {
		a.err = err.Error()
		return a, nil
	}
	a.status = "note added to " + t.Key
	return a, loadIndex(a.vaultPath, a.loc)
}

// cycleAreaOnSelected advances a ticket's area to the next known value.
//
// Setting the area is the whole fix for a stranded ticket: the vault's own
// automation archives it once the property is filled in, so this writes the
// property and lets that automation do the move.
func (a App) cycleAreaOnSelected() (tea.Model, tea.Cmd) {
	t, ok := a.selectedTicket()
	if !ok {
		return a, nil
	}
	areas := a.idx.AreaNames()
	if len(areas) == 0 {
		a.err = "no areas defined in this vault"
		return a, nil
	}
	next := areas[0]
	for i, name := range areas {
		if strings.EqualFold(name, t.Area) {
			next = areas[(i+1)%len(areas)]
			break
		}
	}
	if err := vault.SetProperty(t.Path, "area", next); err != nil {
		a.err = err.Error()
		return a, nil
	}
	a.status = fmt.Sprintf("%s filed under %s", t.Key, next)
	return a, loadIndex(a.vaultPath, a.loc)
}

// openSelectedInObsidian focuses the note in the desktop app.
func (a App) openSelectedInObsidian() (tea.Model, tea.Cmd) {
	t, ok := a.selectedTicket()
	if !ok {
		return a, nil
	}
	if !a.obs.Available() {
		a.err = a.obs.Unavailable()
		return a, nil
	}
	path := t.Path
	a.busy = "opening"
	return a, runAction("open", func(ctx context.Context) (string, error) {
		return a.obs.Open(ctx, path)
	})
}

// jiraAction dispatches a plugin command against the selected ticket.
func (a App) jiraAction(label string, fn func(*obsidian.Client, context.Context, string) (string, error)) (tea.Model, tea.Cmd) {
	if !a.obs.Available() {
		a.err = a.obs.Unavailable()
		return a, nil
	}
	path := ""
	if t, ok := a.selectedTicket(); ok {
		path = t.Path
	}
	client := a.obs
	a.busy = label
	a.status = label + "..."
	return a, runAction(label, func(ctx context.Context) (string, error) {
		return fn(client, ctx, path)
	})
}

// flushWorklog writes tracked time into the note, and only then — if the user
// has opted in — on to Jira.
//
// The local write always happens first. Tracked time is the user's own record;
// it should survive whether or not the Jira call is enabled, reachable, or
// successful.
func (a App) flushWorklog() (tea.Model, tea.Cmd) {
	t, ok := a.selectedTicket()
	if !ok || a.wl == nil {
		return a, nil
	}
	entry := a.wl.Get(t.Key)
	pending := entry.Pending()
	if pending <= 0 {
		a.status = "no tracked time for " + t.Key
		return a, nil
	}
	amount := worklog.Format(pending)
	line := fmt.Sprintf("- %s — %s tracked", time.Now().In(a.loc).Format("2006-01-02 15:04"), amount)
	if err := vault.AppendUnderHeading(t.Path, worklogHeading, line); err != nil {
		a.err = err.Error()
		return a, nil
	}
	a.wl.MarkFlushed(t.Key, pending)
	_ = a.wl.Save()
	a.status = fmt.Sprintf("logged %s to %s", amount, t.Key)

	if !a.worklogPush {
		return a, loadIndex(a.vaultPath, a.loc)
	}
	if !a.obs.Available() {
		a.err = a.obs.Unavailable()
		return a, loadIndex(a.vaultPath, a.loc)
	}
	// The batch command reads its input from a frontmatter property instead
	// of prompting, which is what lets this run without a modal.
	if err := vault.SetProperty(t.Path, "jira_worklog_batch", amount); err != nil {
		a.err = err.Error()
		return a, loadIndex(a.vaultPath, a.loc)
	}
	client := a.obs
	path := t.Path
	a.busy = "worklog to Jira"
	return a, tea.Batch(
		runAction("worklog to Jira", func(ctx context.Context) (string, error) {
			return client.PushWorklogBatch(ctx, path)
		}),
		loadIndex(a.vaultPath, a.loc),
	)
}

// creditPomodoro banks elapsed work time against the bound ticket.
//
// Only the delta since the last tick is added, so a timer that runs for twenty
// minutes credits twenty minutes rather than the sum of every tick's total.
func (a *App) creditPomodoro() {
	if a.pomoKey == "" || a.wl == nil {
		return
	}
	elapsed := a.pomo.workElapsed()
	if elapsed < a.pomoCounted {
		// The phase restarted, so the running total starts over too.
		a.pomoCounted = 0
	}
	if elapsed <= a.pomoCounted {
		return
	}
	delta := elapsed - a.pomoCounted
	a.pomoCounted = elapsed
	a.wl.Add(a.pomoKey, delta, time.Now().In(a.loc))
	_ = a.wl.Save()
}
