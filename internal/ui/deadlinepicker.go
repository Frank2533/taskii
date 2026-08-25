package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"taskii/internal/deadline"
	"taskii/internal/vault"
)

// deadlinePicker is the one-keystroke way to set a deadline, for when typing a
// token per task is more ceremony than it is worth.
type deadlinePicker struct {
	open bool
}

// applyDeadline sets or clears the deadline on the task under the cursor.
//
// It acts on the stored task by ID rather than on the row copy, since the
// lists are sorted and filtered views.
func (a *App) applyDeadline(due *time.Time, remind *time.Duration) bool {
	rows := a.todayRows()
	var id string
	switch a.focus {
	case focusToday:
		if a.todaySelected >= 0 && a.todaySelected < len(rows) {
			r := rows[a.todaySelected]
			if r.isSub {
				// A subtask's schedule belongs in the note, written in the
				// conventions Obsidian plugins already read — otherwise the
				// deadline would exist only inside taskii while the task it
				// belongs to lives in the vault.
				return a.applySubtaskDeadline(r, due, remind)
			}
			id = r.task.ID
		}
	case focusOverdue:
		list := a.overdueTasks()
		if a.overdueSelected >= 0 && a.overdueSelected < len(list) {
			id = list[a.overdueSelected].ID
		}
	}
	if id == "" {
		return false
	}
	for i := range a.tasks {
		if a.tasks[i].ID != id {
			continue
		}
		if due == nil {
			a.tasks[i].Due = nil
		} else {
			d := *due
			a.tasks[i].Due = &d
		}
		if remind != nil {
			at := a.now().Add(*remind)
			a.tasks[i].RemindAt = &at
			// A freshly set reminder has not fired yet, even if an older one
			// on the same task had.
			a.tasks[i].Reminded = false
		}
		a.persist()
		return true
	}
	return false
}

// applySubtaskDeadline writes a subtask's schedule into its own task line.
func (a *App) applySubtaskDeadline(r todayRow, due *time.Time, remind *time.Duration) bool {
	meta := vault.TaskMeta{
		Due: r.sub.Due, HasDue: r.sub.HasDue,
		RemindAt: r.sub.RemindAt, HasRemind: r.sub.HasRemind,
	}
	if due != nil {
		meta.Due, meta.HasDue = *due, true
	} else if remind == nil {
		// Clearing: a bare "clear" removes both, since the picker offers no
		// way to clear them separately.
		meta = vault.TaskMeta{}
	}
	if remind != nil {
		meta.RemindAt, meta.HasRemind = a.now().Add(*remind), true
	}
	if err := vault.SetTaskSchedule(r.ticketPath, r.sub.Line, r.sub.Text, meta, a.loc); err != nil {
		a.err = err.Error()
		return false
	}
	a.needsReindex = true
	return true
}

// updateDeadlinePicker maps one keystroke to a deadline.
func (a App) updateDeadlinePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	now := a.now()
	key := msg.String()

	switch key {
	case "esc", "d", "q":
		a.picker.open = false
		return a, nil
	case "x":
		a.applyDeadline(nil, nil)
		a.picker.open = false
		a.status = "deadline cleared"
		return a, a.reindexIfNeeded()
	case "t":
		return a.setPickerDue(deadline.InDays(now, 0), "today")
	case "m":
		return a.setPickerDue(deadline.InDays(now, 1), "tomorrow")
	case "w":
		return a.setPickerDue(deadline.EndOfWeek(now), "end of week")
	case "c":
		return a.beginReminderForm()
	case "h":
		d := time.Hour
		a.applyDeadline(nil, &d)
		a.picker.open = false
		a.status = "reminder in 1 hour"
		return a, a.reindexIfNeeded()
	}

	// 1..9 set a deadline that many days out; 0 is today.
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		n := int(key[0] - '0')
		return a.setPickerDue(deadline.InDays(now, n), deadline.Short(deadline.InDays(now, n), now))
	}
	return a, nil
}

func (a App) setPickerDue(due time.Time, label string) (tea.Model, tea.Cmd) {
	if a.applyDeadline(&due, nil) {
		a.status = "due " + label
	}
	a.picker.open = false
	return a, a.reindexIfNeeded()
}

// reindexIfNeeded rebuilds the index when the last action wrote to a note, so
// the two views agree about what is in it.
func (a *App) reindexIfNeeded() tea.Cmd {
	if !a.needsReindex {
		return nil
	}
	a.needsReindex = false
	return loadIndex(a.vaultPath, a.loc)
}

func (a App) renderDeadlinePicker() string {
	muted := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	text := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)
	key := lipgloss.NewStyle().Foreground(colorAccent).Background(colorPaneBg).Bold(true)

	row := func(k, label string) string {
		return key.Render("  "+k+"  ") + text.Render(label)
	}
	lines := []string{
		muted.Render("  Deadline"),
		row("t", "today"),
		row("m", "tomorrow"),
		row("1-9", "in N days"),
		row("w", "end of this week"),
		row("x", "clear deadline"),
		"",
		muted.Render("  Start reminder"),
		row("h", "in 1 hour"),
		row("c", "custom: days, hours, minutes"),
		"",
		muted.Render("  esc  cancel"),
	}
	width := 34
	return renderPane("Set deadline", strings.Join(lines, "\n"), true, width, len(lines)+2)
}
