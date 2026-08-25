package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"taskii/internal/model"
	"taskii/internal/vault"
)

// noteTarget is where a note about the current selection should be written.
//
// Three destinations, because a note means a different thing depending on what
// it is about: a note on one subtask belongs attached to that line, a note on a
// ticket belongs in its work log, and a note on a local task belongs in the
// body of its own note.
type noteTarget struct {
	kind noteTargetKind
	name string // what the note is about, for the pane title

	path string // the vault note to write into
	line int    // the task line, for a subtask
	text string // that task's text, to detect a stale line

	existing []string
}

type noteTargetKind int

const (
	noteTargetNone noteTargetKind = iota
	noteTargetSubtask
	noteTargetTicket
	noteTargetLocalTask
	// noteTargetUnsynced is a task with no note in the vault yet, because
	// Obsidian sync is off. Its notes have nowhere to live.
	noteTargetUnsynced
)

// noteTarget resolves the selection in the task panes.
func (a App) noteTarget() noteTarget {
	if a.focus != focusToday && a.focus != focusOverdue {
		return noteTarget{}
	}

	if a.focus == focusToday {
		rows := a.todayRows()
		if a.todaySelected >= 0 && a.todaySelected < len(rows) {
			if r := rows[a.todaySelected]; r.isSub {
				return noteTarget{
					kind: noteTargetSubtask, name: r.sub.Text,
					path: r.ticketPath, line: r.sub.Line, text: r.sub.Text,
					existing: r.sub.Notes,
				}
			}
		}
	}

	t := a.selectedTask()
	if t == nil {
		return noteTarget{}
	}

	if t.IsTicket() {
		if a.idx == nil {
			return noteTarget{kind: noteTargetUnsynced, name: t.Title}
		}
		tk, ok := a.idx.Ticket(t.TicketKey)
		if !ok {
			return noteTarget{kind: noteTargetUnsynced, name: t.Title}
		}
		return noteTarget{
			kind: noteTargetTicket, name: tk.Key,
			path: tk.Path, existing: tk.Notes,
		}
	}

	path := a.localTaskNotePath(*t)
	if path == "" {
		return noteTarget{kind: noteTargetUnsynced, name: t.Title}
	}
	return noteTarget{
		kind: noteTargetLocalTask, name: t.Title,
		path: path, existing: a.localTaskNotes(*t),
	}
}

// localTaskNotePath finds a local task's note, re-resolving by id when the
// remembered path is wrong.
func (a App) localTaskNotePath(t model.Task) string {
	if !a.syncEnabled() {
		return ""
	}
	if t.NotePath != "" {
		return t.NotePath
	}
	found, err := vault.FindNoteByID(a.projectDir(), t.ID)
	if err != nil {
		return ""
	}
	return found
}

// localTaskNotes reads a local task's note body out of the index.
func (a App) localTaskNotes(t model.Task) []string {
	if a.idx == nil {
		return nil
	}
	for _, lt := range a.idx.LocalTasks {
		if lt.ID == t.ID {
			return lt.Notes
		}
	}
	return nil
}

// heading is the section a note goes under, for the targets that use one.
func (n noteTarget) heading() string {
	switch n.kind {
	case noteTargetTicket:
		return worklogHeading
	case noteTargetLocalTask:
		return "Notes"
	default:
		return ""
	}
}

// describe names the destination, so it is clear where a note will land.
func (n noteTarget) describe() string {
	switch n.kind {
	case noteTargetSubtask:
		return "under this subtask"
	case noteTargetTicket:
		return worklogHeading
	case noteTargetLocalTask:
		return "the task's note"
	default:
		return ""
	}
}

// beginTaskNote opens the input for a note about the selection.
func (a App) beginTaskNote() (tea.Model, tea.Cmd) {
	target := a.noteTarget()
	switch target.kind {
	case noteTargetNone:
		return a, nil
	case noteTargetUnsynced:
		a.setErr("notes on a task need Obsidian sync — turn it on in settings (,)")
		return a, nil
	}
	a.noteFor = target
	a.mode = modeTaskNote
	a.input.SetValue("")
	a.input.Placeholder = "note for " + target.name
	a.input.Focus()
	return a, nil
}

// updateTaskNote drives the note input.
func (a App) updateTaskNote(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.mode = modeNormal
		a.noteFor = noteTarget{}
		a.input.Blur()
		a.input.SetValue("")
		return a, nil
	case "enter":
		target := a.noteFor
		text := strings.TrimSpace(a.input.Value())
		a.mode = modeNormal
		a.noteFor = noteTarget{}
		a.input.Blur()
		a.input.SetValue("")
		if text == "" {
			return a, nil
		}

		var err error
		switch target.kind {
		case noteTargetSubtask:
			err = vault.AppendNoteUnderCheckbox(target.path, target.line, target.text, text)
		default:
			// Dated, because a work log is read as a history.
			line := fmt.Sprintf("- %s — %s", time.Now().In(a.loc).Format("2006-01-02 15:04"), text)
			err = vault.AppendUnderHeading(target.path, target.heading(), line)
		}
		if err != nil {
			a.setErr(err.Error())
			return a, loadIndex(a.vaultPath, a.loc)
		}
		a.setStatus("note added to " + target.name)
		return a, loadIndex(a.vaultPath, a.loc)
	}
	a.input.Width = a.inputFieldWidth()
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return a, cmd
}

// renderTaskNotes draws the selection's notes in the Notes pane.
func (a App) renderTaskNotes(target noteTarget, width, height int) string {
	muted := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	text := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)

	if target.kind == noteTargetUnsynced {
		return muted.Render(fitToWidth("Turn on Obsidian sync in settings to keep notes on this task.", width))
	}
	if len(target.existing) == 0 {
		return muted.Render(fitToWidth("No notes yet — press N to add one.", width))
	}

	var rows []string
	shown := 0
	for _, n := range target.existing {
		if len(rows) >= height {
			break
		}
		// Long notes wrap rather than being cut off: a note is prose, and the
		// end of a sentence is usually the part that matters. Only the first
		// line of each carries the bullet.
		for i, chunk := range wrapText(n, width-2) {
			if len(rows) >= height {
				break
			}
			marker := "  "
			if i == 0 {
				marker = "• "
			}
			rendered := text.Render(fitToWidth(marker+chunk, width))
			// Any URL pasted into a note becomes clickable in place, since
			// this is exactly the kind of freeform text people paste links
			// into.
			rows = append(rows, linkifyURLs(rendered))
		}
		shown++
	}
	if rest := len(target.existing) - shown; rest > 0 {
		if len(rows) >= height && height > 0 {
			rows = rows[:height-1]
		}
		rows = append(rows, muted.Render(fmt.Sprintf("+%d more", rest)))
	}
	return strings.Join(rows, "\n")
}

// wrapText breaks a note into lines of at most width, on word boundaries.
func wrapText(s string, width int) []string {
	if width < 8 {
		width = 8
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var out []string
	line := words[0]
	for _, w := range words[1:] {
		if len(line)+1+len(w) > width {
			out = append(out, line)
			line = w
			continue
		}
		line += " " + w
	}
	return append(out, line)
}

// renderTaskNoteInput draws the note prompt inside the Notes pane, which is
// where the note is about to appear.
func (a App) renderTaskNoteInput(width int) string {
	a.input.TextStyle = lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)
	a.input.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent).Background(colorPaneBg)
	a.input.Cursor.Style = lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)
	a.input.Width = 0

	hint := ""
	if d := a.noteFor.describe(); d != "" {
		hint = lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg).
			Render(fitToWidth("→ "+d, width))
	}
	line := inputPromptStyle.Render("+ ") + a.input.View()
	if hint == "" {
		return line
	}
	return hint + "\n" + line
}
