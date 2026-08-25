package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// keyRow is one binding in the "?" overlay.
type keyRow struct{ key, what string }

// keySection groups bindings under a heading.
type keySection struct {
	name string
	rows []keyRow
}

// keyReference is every binding for the current view.
//
// The bottom bar can only carry a handful of keys before it wraps into the
// content, so most bindings were reachable only by knowing they existed. This
// is the full list, per view, so nothing is undiscoverable.
func (a App) keyReference() []keySection {
	common := keySection{"App", []keyRow{
		{"1 / 2 / 3", "dashboard / PARA / calendar"},
		{",", "settings"},
		{"?", "this list"},
		{"t", "cycle theme"},
		{"L", "cycle layout"},
		{"q", "quit"},
	}}

	switch a.view {
	case viewPARA:
		sections := []keySection{
			{"Navigate", []keyRow{
				{"tab / shift+tab", "move between panes"},
				{"↑ ↓ / j k", "move the cursor"},
				{"enter", "toggle a task, or step to the next pane"},
			}},
			{"Vault", []keyRow{
				{"a", "add a task to the selected note"},
				{"n", "add a dated note to the ticket"},
				{"u", "file the ticket under the next area"},
				{"o", "open the note in Obsidian"},
				{"r", "reindex the vault"},
				{"C", "export the calendar now"},
			}},
		}
		if a.jiraEnabled {
			sections = append(sections, keySection{"Jira", []keyRow{
				{"R", "fetch issues"},
				{"s", "start a status transition"},
				{"c", "add a comment"},
				{"p", "track pomodoro time against this ticket"},
				{"w", "write tracked time to the work log"},
			}})
		} else {
			sections = append(sections, keySection{"Time", []keyRow{
				{"p", "track pomodoro time against this ticket"},
				{"w", "write tracked time to the work log"},
			}})
		}
		return append(sections, common)
	case viewCalendar:
		return []keySection{
			{"Calendar", []keyRow{
				{"w / m / y", "week, month or year"},
				{"← →  h l", "move a day (a month at year scale)"},
				{"↑ ↓  k j", "move a week (a quarter at year scale)"},
				{"[ ]", "previous / next period"},
				{"T", "jump to today"},
				{"tab", "step through the day's entries"},
				{"a / e / d", "add, edit or delete an event"},
				{"", "editing a repeat asks: this / this and future / all"},
				{"r", "reindex the vault"},
				{"C", "export the calendar now"},
			}},
			{"Reminders", []keyRow{
				{"automatic", "15, 5 and 1 minutes before every event"},
				{",", "settings: enable phone push and set an ntfy topic"},
				{"", "history: run  taskii notifications"},
			}},
			{"Event syntax", []keyRow{
				{"09:30-10:00", "start and end (required)"},
				{"!tmr !2d !fri", "which day (today if omitted)"},
				{"daily weekly", "monthly, yearly"},
				{"weekdays", "Mon-Fri (implies weekly)"},
				{"mon,wed,fri", "specific days (implies weekly)"},
				{"x2", "every other period"},
			}},
			common,
		}
	}

	return []keySection{
		{"Tasks", []keyRow{
			{"a", "add a task (type a ticket summary to attach one)"},
			{"A", "add a subtask to the selected ticket"},
			{"e", "edit the task or subtask under the cursor"},
			{"space / enter", "toggle done"},
			{"d", "delete"},
			{"i", "mark important"},
			{"D", "set a deadline or start reminder"},
			{"D then c", "custom reminder: days, hours, minutes"},
			{"z", "fold a ticket's subtasks"},
		}},
		{"Filter and move", []keyRow{
			{"tab / shift+tab", "move between panes"},
			{"↑ ↓ / j k", "move the cursor"},
			{"I", "show only important"},
			{"U", "show only unfinished"},
		}},
		{"Deadlines", []keyRow{
			{"!today !tmr", "due today / tomorrow"},
			{"!2d !fri !w", "in N days / a weekday / end of week"},
			{"@3h @90m", "remind me to start, from now"},
		}},
		{"Pomodoro", []keyRow{
			{"p", "start or pause"},
			{"r", "reset the phase"},
			{"n", "skip the phase"},
		}},
		common,
	}
}

func (a App) renderKeyHelp() string {
	width := a.width - 8
	if width > 92 {
		width = 92
	}
	if width < 34 {
		width = 34
	}
	height := a.height - a.chromeLines()

	heading := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	key := lipgloss.NewStyle().Foreground(colorAccent).Background(colorPaneBg).Bold(true)
	text := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)

	var lines []string
	for _, sec := range a.keyReference() {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, heading.Render("  "+sec.name))
		for _, r := range sec.rows {
			lines = append(lines, key.Render("  "+pad(r.key, 18))+text.Render(fitToWidth(r.what, width-22)))
		}
	}
	// The overlay is a reference, not a scrolling list: if it cannot fit, the
	// terminal is too short and truncating is clearer than a partial scroll.
	if height > 2 && len(lines) > height-2 {
		lines = lines[:height-2]
	}
	return renderPane(a.view.String()+" — all keys", strings.Join(lines, "\n"), true, width, len(lines)+2)
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s + " "
	}
	return s + strings.Repeat(" ", n-len(s))
}
