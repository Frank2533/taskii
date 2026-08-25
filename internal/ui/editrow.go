package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"taskii/internal/deadline"
	"taskii/internal/vault"
)

// editTarget remembers what an in-progress edit applies to.
//
// The row itself is not kept: the lists are sorted, filtered views and an
// index would no longer point at the same thing once the edit lands.
type editTarget struct {
	taskID string // a task row

	subPath string // a subtask line in a vault note
	subLine int
	subText string
}

func (e editTarget) isSub() bool { return e.subPath != "" }
func (e editTarget) empty() bool { return e.taskID == "" && e.subPath == "" }

// beginEditRow opens the input primed with whatever the cursor is on.
func (a App) beginEditRow() (tea.Model, tea.Cmd) {
	var target editTarget
	var seed string

	switch a.focus {
	case focusToday:
		rows := a.todayRows()
		if a.todaySelected < 0 || a.todaySelected >= len(rows) {
			return a, nil
		}
		r := rows[a.todaySelected]
		if r.isSub {
			target = editTarget{subPath: r.ticketPath, subLine: r.sub.Line, subText: r.sub.Text}
			seed = r.sub.Text
		} else {
			target = editTarget{taskID: r.task.ID}
			seed = r.task.Title
		}
	case focusOverdue:
		list := a.overdueTasks()
		if a.overdueSelected < 0 || a.overdueSelected >= len(list) {
			return a, nil
		}
		target = editTarget{taskID: list[a.overdueSelected].ID}
		seed = list[a.overdueSelected].Title
	default:
		return a, nil
	}

	a.editing = target
	a.mode = modeEditRow
	a.input.SetValue(seed)
	a.input.Placeholder = "edit, then enter"
	a.input.CursorEnd()
	a.input.Focus()
	return a, nil
}

// updateEditRow drives the edit input.
func (a App) updateEditRow(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.mode = modeNormal
		a.editing = editTarget{}
		a.input.Blur()
		a.input.SetValue("")
		return a, nil
	case "enter":
		return a.commitEditRow(a.input.Value())
	}
	a.input.Width = a.inputFieldWidth()
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return a, cmd
}

// commitEditRow applies the edit.
func (a App) commitEditRow(raw string) (tea.Model, tea.Cmd) {
	target := a.editing
	a.mode = modeNormal
	a.editing = editTarget{}
	a.input.Blur()
	a.input.SetValue("")

	text := strings.TrimSpace(raw)
	if text == "" || target.empty() {
		return a, nil
	}

	if target.isSub() {
		// A subtask lives in the vault note, so the edit goes there and the
		// index is rebuilt from what actually landed.
		if err := vault.SetCheckboxText(target.subPath, target.subLine, target.subText, text); err != nil {
			a.err = err.Error()
		}
		return a, loadIndex(a.vaultPath, a.loc)
	}

	// Editing a title may also carry deadline tokens, so the same syntax
	// works whether a task is being created or corrected.
	title, spec := deadline.Parse(text, a.now())
	title = strings.TrimSpace(title)
	if title == "" {
		return a, nil
	}
	for i := range a.tasks {
		if a.tasks[i].ID != target.taskID {
			continue
		}
		a.tasks[i].Title = title
		if spec.HasDue {
			due := spec.Due
			a.tasks[i].Due = &due
		}
		if spec.HasRemind {
			at := spec.RemindAt
			a.tasks[i].RemindAt = &at
			a.tasks[i].Reminded = false
		}
		a.persist()
		a.syncLocalTask(a.tasks[i].ID)
		break
	}
	return a, nil
}

// beginAddSubtask starts a new task line on the ticket under the cursor.
func (a App) beginAddSubtask() (tea.Model, tea.Cmd) {
	rows := a.todayRows()
	if a.todaySelected < 0 || a.todaySelected >= len(rows) {
		return a, nil
	}
	r := rows[a.todaySelected]
	if !r.task.IsTicket() {
		a.err = "subtasks belong to a ticket — select one first"
		return a, nil
	}
	path := r.ticketPath
	if path == "" && a.idx != nil {
		if tk, ok := a.idx.Ticket(r.task.TicketKey); ok {
			path = tk.Path
		}
	}
	if path == "" {
		a.err = "could not find that ticket's note"
		return a, nil
	}
	a.editing = editTarget{subPath: path, subLine: -1}
	a.mode = modeAddSubtask
	a.input.SetValue("")
	a.input.Placeholder = "new subtask for " + r.task.TicketKey
	a.input.Focus()
	return a, nil
}

// updateAddSubtask drives the new-subtask input.
func (a App) updateAddSubtask(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.mode = modeNormal
		a.editing = editTarget{}
		a.input.Blur()
		a.input.SetValue("")
		return a, nil
	case "enter":
		target := a.editing
		text := strings.TrimSpace(a.input.Value())
		a.mode = modeNormal
		a.editing = editTarget{}
		a.input.Blur()
		a.input.SetValue("")
		if text == "" {
			return a, nil
		}
		if err := vault.AppendCheckbox(target.subPath, quickAddHeading, text); err != nil {
			a.err = err.Error()
			return a, nil
		}
		return a, loadIndex(a.vaultPath, a.loc)
	}
	a.input.Width = a.inputFieldWidth()
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return a, cmd
}
