package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// agendaEntry is one dated item, from the vault or from local tasks.
type agendaEntry struct {
	when    time.Time
	allDay  bool
	title   string
	detail  string
	overdue bool
}

// agenda merges vault dates with taskii's own appointments, oldest first.
//
// Day boundaries are computed in the configured zone rather than the machine's,
// so "today" means the user's today.
func (a App) agenda() []agendaEntry {
	loc := a.loc
	if loc == nil {
		loc = time.Local
	}
	today := startOfDay(a.now().In(loc))

	var out []agendaEntry
	if a.idx != nil {
		for _, e := range a.idx.Events() {
			day := startOfDay(e.Date.In(loc))
			out = append(out, agendaEntry{
				when:    day,
				allDay:  true,
				title:   e.Title,
				detail:  strings.TrimSpace(e.Kind + " " + e.Status),
				overdue: day.Before(today),
			})
		}
	}
	for _, t := range a.tasks {
		if !t.IsAppointment() || t.Done || t.Date == "" || t.Time == "" {
			continue
		}
		ts, err := time.ParseInLocation("2006-01-02 15:04", t.Date+" "+t.Time, loc)
		if err != nil {
			continue
		}
		out = append(out, agendaEntry{
			when:    ts,
			title:   t.Title,
			detail:  "appointment",
			overdue: ts.Before(a.now().In(loc)),
		})
	}
	sortAgenda(out)
	return out
}

func sortAgenda(entries []agendaEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].when.Before(entries[j-1].when); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func (a App) renderCalendar() string {
	height := a.height - a.chromeLines()
	if height < 3 {
		height = 3
	}
	width := a.width

	if line := a.vaultStatusLine(); line != "" && a.idx == nil {
		return renderPane("Calendar", lipgloss.NewStyle().Foreground(colorWarning).Render(line), true, width, height)
	}

	entries := a.agenda()
	if len(entries) == 0 {
		body := lipgloss.NewStyle().Foreground(colorMuted).
			Render("Nothing scheduled. Due dates on tickets and timed tasks appear here.")
		return renderPane("Calendar", body, true, width, height)
	}

	loc := a.loc
	if loc == nil {
		loc = time.Local
	}
	today := startOfDay(a.now().In(loc))

	muted := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	text := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)

	var rows []string
	lastDay := time.Time{}
	for _, e := range entries {
		if len(rows) >= height-2 {
			break
		}
		day := startOfDay(e.when)
		if !day.Equal(lastDay) {
			lastDay = day
			label := day.Format("Mon 02 Jan")
			switch {
			case day.Equal(today):
				label += "  today"
			case day.Before(today):
				label += fmt.Sprintf("  %d days ago", int(today.Sub(day).Hours()/24))
			}
			style := muted
			if day.Before(today) {
				style = lipgloss.NewStyle().Foreground(colorDanger).Background(colorPaneBg)
			} else if day.Equal(today) {
				style = lipgloss.NewStyle().Foreground(colorAccent).Background(colorPaneBg).Bold(true)
			}
			if len(rows) > 0 {
				rows = append(rows, "")
			}
			rows = append(rows, style.Render(fitToWidth(label, width-4)))
		}

		when := "     "
		if !e.allDay {
			when = e.when.Format("15:04")
		}
		style := text
		if e.overdue {
			style = style.Foreground(colorWarning)
		}
		rows = append(rows, style.Render(fitToWidth("  "+when+"  "+e.title, width-4)))
	}

	title := fmt.Sprintf("Calendar (%d) — %s", len(entries), loc)
	return renderPane(title, strings.Join(rows, "\n"), true, width, height)
}

// upcomingCount is used by the header to hint at what the calendar holds.
func (a App) upcomingCount() int {
	n := 0
	for _, e := range a.agenda() {
		if !e.overdue {
			n++
		}
	}
	return n
}
