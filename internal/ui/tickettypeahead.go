package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"taskii/internal/deadline"
	"taskii/internal/model"
	"taskii/internal/para"
)

// maxSuggestions bounds the typeahead list so it never crowds out the task
// list it sits under.
const maxSuggestions = 5

// addSuggestions matches what is being typed against ticket summaries.
//
// Tickets are never mirrored into Today wholesale — the pane is a working set
// the user chooses, not a feed of everything in the vault — so this is the way
// a ticket gets in.
func (a App) addSuggestions() []para.Match {
	if a.idx == nil || a.mode != modeAdding {
		return nil
	}
	// Deadline tokens are stripped first so typing "!tmr" mid-phrase does not
	// poison the search text.
	query, _ := deadline.Parse(a.input.Value(), a.now())
	query = strings.TrimSpace(query)
	if len(query) < 2 {
		return nil
	}
	matches := a.idx.Search(query)
	// Only tickets: a project has no key to attach a task row to.
	var out []para.Match
	for _, m := range matches {
		if m.Kind != "ticket" || m.Key == "" {
			continue
		}
		if a.hasTicketRow(m.Key) {
			continue
		}
		// Archived and closed tickets are finished work; offering them as
		// something to put on today's list is never what was meant.
		if tk, ok := a.idx.Ticket(m.Key); ok && (tk.Archived || tk.Closed()) {
			continue
		}
		out = append(out, m)
		if len(out) == maxSuggestions {
			break
		}
	}
	return out
}

// suggestionStatus is the ticket's status, shown so a suggestion can be told
// apart from a similarly-named one without opening it.
func (a App) suggestionStatus(key string) string {
	if a.idx == nil {
		return ""
	}
	if tk, ok := a.idx.Ticket(key); ok {
		return tk.Status
	}
	return ""
}

// hasTicketRow reports whether a ticket is already on today's list, so the
// suggestions never offer a duplicate.
func (a App) hasTicketRow(key string) bool {
	today := a.now().Format(dateFormat)
	for _, t := range a.tasks {
		if t.TicketKey == key && t.Date == today && !t.Done {
			return true
		}
	}
	return false
}

// addTicketTask puts a chosen ticket on today's list.
func (a *App) addTicketTask(m para.Match, spec deadline.Spec) {
	t := model.Task{
		ID:        strconv.FormatInt(a.now().UnixNano(), 36),
		Title:     m.Label,
		Kind:      model.KindTask,
		Date:      a.now().Format(dateFormat),
		CreatedAt: a.now(),
		TicketKey: m.Key,
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
	a.selectTaskByID(t.ID)
}

// renderSuggestions draws the typeahead list beneath the input.
func (a App) renderSuggestions(matches []para.Match, width int) string {
	if len(matches) == 0 {
		return ""
	}
	muted := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	sel := lipgloss.NewStyle().Foreground(colorAccent).Background(colorPanel).Bold(true)

	var lines []string
	for i, m := range matches {
		style := muted
		prefix := "    "
		if i == a.addSuggest {
			style = sel
			prefix = "  > "
		}
		label := m.Label
		if st := a.suggestionStatus(m.Key); st != "" {
			label += "  [" + st + "]"
		}
		lines = append(lines, style.Render(fitToWidth(prefix+label, width)))
	}
	return strings.Join(lines, "\n")
}
