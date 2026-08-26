package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"taskii/internal/deadline"
	"taskii/internal/model"
	"taskii/internal/para"
	"taskii/internal/vault"
)

// todayRow is one line in the Today pane. A row is either a task — local or
// standing for a vault ticket — or one of a ticket's subtasks.
//
// The pane addresses rows rather than tasks because a ticket contributes a
// variable number of lines, so a task index would no longer identify what the
// cursor is on.
type todayRow struct {
	task  model.Task
	isSub bool
	sub   para.Checkbox

	// ticketPath is the note a subtask lives in, resolved at build time from
	// the ticket key rather than remembered, since the vault renames these
	// files from their properties on a debounce.
	ticketPath string
}

// todayRows expands today's tasks, inserting each ticket's open subtasks
// beneath it unless that ticket is collapsed.
func (a App) todayRows() []todayRow {
	return a.rowsFor(focusToday)
}

// overdueRows is the same expansion over the Overdue list.
//
// Overdue used to render as a flat list built straight from model.Task, with
// no subtask expansion at all — so a carried-over ticket showed none of its
// tasks, and z, A, N on a subtask, and edit-a-subtask had nothing to act on
// once a task left Today. There was only ever one reason to keep the two
// separate: Today alone reserves room for a fully general "type a title,
// pick a ticket from the typeahead" quick-add box. Everything else — the
// expansion, the toggle/fold/edit/note behaviour — is the same operation
// regardless of which pane the row happens to be sitting in today.
func (a App) overdueRows() []todayRow {
	return a.rowsFor(focusOverdue)
}

// rowsFor expands the given pane's task list, inserting each ticket's open
// subtasks beneath it unless that ticket is collapsed.
func (a App) rowsFor(focus focusedPane) []todayRow {
	var tasks []model.Task
	switch focus {
	case focusOverdue:
		tasks = a.overdueTasks()
	default:
		tasks = a.todayTasks()
	}
	rows := make([]todayRow, 0, len(tasks))
	for _, t := range tasks {
		rows = append(rows, todayRow{task: t})
		if !t.IsTicket() || t.Collapsed || a.idx == nil {
			continue
		}
		ticket, ok := a.idx.Ticket(t.TicketKey)
		if !ok {
			continue
		}
		for _, c := range ticket.Checkboxes {
			rows = append(rows, todayRow{
				task:       t,
				isSub:      true,
				sub:        c,
				ticketPath: ticket.Path,
			})
		}
	}
	return rows
}

// subtaskCounts summarises a ticket row's progress for its own line.
func (a App) subtaskCounts(key string) (done, total int) {
	if a.idx == nil || key == "" {
		return 0, 0
	}
	t, ok := a.idx.Ticket(key)
	if !ok {
		return 0, 0
	}
	for _, c := range t.Checkboxes {
		total++
		if c.Done {
			done++
		}
	}
	return done, total
}

// renderTodayRows draws the expanded list. overdue applies the same red
// styling renderTaskList used for the flat Overdue list, so switching Overdue
// to the row-based renderer did not lose the visual cue that a task or ticket
// is late.
func (a App) renderTodayRows(rows []todayRow, selected, scroll, visible int, focused, overdue bool, width int) string {
	if len(rows) == 0 {
		return hintStyle.Render("(no tasks)") + "\n"
	}
	if visible < 1 {
		visible = 1
	}
	end := scroll + visible
	if end > len(rows) {
		end = len(rows)
	}

	now := a.now()
	var lines []string
	for i := scroll; i < end; i++ {
		r := rows[i]
		if !r.isSub {
			t := r.task
			if t.IsTicket() {
				t.Title = a.ticketRowTitle(t)
			}
			if t.HasDue() {
				t.Title += "  " + deadlineMarker(t, now)
			}
			lines = append(lines, renderTaskLine(t, i == selected && focused, overdue, width, colorPaneBg))
			continue
		}
		lines = append(lines, a.renderSubtaskLine(r, i == selected && focused, width))
	}

	above, below := scroll, len(rows)-end
	indicator := ""
	switch {
	case above > 0 && below > 0:
		indicator = "↑ " + itoa(above) + " more / ↓ " + itoa(below) + " more"
	case above > 0:
		indicator = "↑ " + itoa(above) + " more"
	case below > 0:
		indicator = "↓ " + itoa(below) + " more"
	}
	lines = append(lines, hintStyle.Render(indicator))
	return strings.Join(lines, "\n")
}

// ticketRowTitle labels a ticket row with its subtask progress and whether it
// is collapsed, so a folded row still reports what it is hiding.
func (a App) ticketRowTitle(t model.Task) string {
	done, total := a.subtaskCounts(t.TicketKey)
	marker := "▾"
	if t.Collapsed {
		marker = "▸"
	}
	if total == 0 {
		return marker + " " + t.Title
	}
	return marker + " " + t.Title + "  " + itoa(done) + "/" + itoa(total)
}

func (a App) renderSubtaskLine(r todayRow, selected bool, width int) string {
	box := "[ ]"
	style := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)
	if r.sub.Done {
		box = "[x]"
		style = lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg).Strikethrough(true)
	}
	prefix := "    "
	if selected {
		prefix = "  > "
		style = style.Background(colorPanel).Bold(true)
	}
	label := r.sub.Text
	if r.sub.HasDue {
		label += "  ⌛ " + deadline.Short(r.sub.Due, a.now())
	}
	return style.Render(fitToWidth(prefix+box+" "+label, width))
}

// toggleTodayRow flips whatever the cursor is on: a subtask writes through to
// the vault note, a task toggles locally.
//
// The write goes to the note rather than to any local copy, so the vault stays
// the single source of truth for a ticket's subtasks and the two views cannot
// drift apart.
func (a App) toggleTodayRow() (App, bool) {
	rows := a.rowsFor(a.focus)
	sel := a.currentSelected()
	if sel < 0 || sel >= len(rows) {
		return a, false
	}
	r := rows[sel]
	if !r.isSub {
		// Toggle by ID, not by index. These panes address rows while the
		// task list addresses tasks, and an expanded ticket makes the two
		// diverge — resolving by position would tick a different task than
		// the one under the cursor.
		a.toggleTaskByID(r.task.ID)
		return a, false
	}
	if err := vault.SetCheckbox(r.ticketPath, r.sub.Line, r.sub.Text, !r.sub.Done); err != nil {
		a.setErr(err.Error())
	}
	// Reindex either way: a refusal means our picture of the note is stale.
	return a, true
}

// selectedRowTaskID is the task under the cursor in a row-based pane (Today
// or Overdue), "" when the cursor is on a subtask or nothing is selected. Row
// and task indices differ once a ticket is expanded, so every action on
// either pane resolves through this rather than indexing the task list
// directly.
func (a App) selectedRowTaskID(focus focusedPane) string {
	rows := a.rowsFor(focus)
	sel := a.currentSelected()
	if a.focus != focus {
		// currentSelected() is keyed off a.focus; asking about a pane that
		// is not focused has no defined selection.
		return ""
	}
	if sel < 0 || sel >= len(rows) {
		return ""
	}
	if rows[sel].isSub {
		return ""
	}
	return rows[sel].task.ID
}

// toggleCollapseTodayRow folds or unfolds the ticket under the cursor.
func (a *App) toggleCollapseTodayRow() {
	rows := a.rowsFor(a.focus)
	sel := a.currentSelected()
	if sel < 0 || sel >= len(rows) {
		return
	}
	r := rows[sel]
	if !r.task.IsTicket() {
		return
	}
	// Collapsing from a subtask row folds its parent. Requiring the cursor to
	// be on the ticket line made the key look broken from the rows where you
	// most want to use it.
	for i := range a.tasks {
		if a.tasks[i].ID == r.task.ID {
			a.tasks[i].Collapsed = !a.tasks[i].Collapsed
			a.persist()
			return
		}
	}
}

// bumpToToday resets the Date on the task under the cursor to today.
//
// This is the only undo there was no path back from before: a carried-over
// task (Date before today, no deadline miss involved) had no way back to
// Today except finishing or deleting it — there was no way to say "no,
// actually, today." A task whose deadline has separately been missed keeps
// that deadline, so it can still be found in Overdue afterwards too; this
// only ever touches which day the task is filed under, the same distinction
// the rest of the deadline machinery in this codebase already draws between
// Date and Due.
func (a *App) bumpToToday() {
	id := a.actionTaskID()
	if id == "" {
		rows := a.rowsFor(a.focus)
		sel := a.currentSelected()
		if sel >= 0 && sel < len(rows) && rows[sel].isSub {
			// A subtask has no Date of its own to bump, only its parent
			// ticket does.
			a.setErr("select the task or ticket itself, not one of its subtasks")
		}
		return
	}
	for i := range a.tasks {
		if a.tasks[i].ID != id {
			continue
		}
		a.tasks[i].Date = a.now().Format(dateFormat)
		a.persist()
		a.syncLocalTask(id)
		a.setStatus("moved back to today")
		return
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
