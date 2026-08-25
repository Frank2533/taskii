package ui

import (
	"fmt"
	"time"

	"taskii/internal/model"
)

// calScale is how much of the calendar is on screen at once.
type calScale int

const (
	calWeek calScale = iota
	calMonth
	calYear

	calScaleCount = 3
)

func (c calScale) String() string {
	switch c {
	case calWeek:
		return "Week"
	case calMonth:
		return "Month"
	default:
		return "Year"
	}
}

// calEntry is one thing on a day: an event, a task, or a ticket.
type calEntry struct {
	at      time.Time
	timed   bool
	title   string
	subs    int // open subtasks, shown in brackets
	kind    timelineKind
	overdue bool
}

// label renders an entry for a day cell.
//
// The subtask count rides in brackets after the title, because a day cell is
// only ever a few characters wide and a separate column for it would cost more
// room than the number is worth.
func (e calEntry) label(showTime bool) string {
	s := e.title
	if e.subs > 0 {
		s = fmt.Sprintf("%s (%d)", s, e.subs)
	}
	if showTime && e.timed {
		s = e.at.Format("15:04") + " " + s
	}
	return s
}

// calendarEntries collects everything dated in [from, to).
func (a App) calendarEntries(from, to time.Time) []calEntry {
	loc := a.loc
	if loc == nil {
		loc = time.Local
	}
	var out []calEntry

	for _, o := range model.EventsOccurring(a.events, from, to) {
		out = append(out, calEntry{
			at: o.Start.In(loc), timed: !o.Event.AllDay,
			title: o.Event.Title, kind: tlEvent,
		})
	}

	for _, t := range a.tasks {
		if t.Done {
			continue
		}
		when, timed, ok := a.taskCalendarTime(t, loc)
		if !ok || when.Before(from) || !when.Before(to) {
			continue
		}
		out = append(out, calEntry{
			at: when, timed: timed, title: t.Title,
			subs: a.openSubtaskCount(t), kind: tlDeadline,
			overdue: t.PastDue(a.now()),
		})
	}

	if a.idx != nil {
		for _, tk := range a.idx.Tickets {
			if !tk.HasDue || tk.Closed() {
				continue
			}
			day := startOfDay(tk.Due.In(loc))
			if day.Before(from) || !day.Before(to) {
				continue
			}
			open := 0
			for _, c := range tk.Checkboxes {
				if !c.Done {
					open++
				}
			}
			out = append(out, calEntry{at: day, title: tk.Title(), subs: open, kind: tlDeadline})
		}
	}

	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].at.Before(out[j-1].at); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// taskCalendarTime is where a task belongs on the calendar: its deadline if it
// has one, otherwise its appointment time.
func (a App) taskCalendarTime(t model.Task, loc *time.Location) (time.Time, bool, bool) {
	if t.HasDue() {
		return t.Deadline().In(loc), false, true
	}
	if t.IsAppointment() && t.Date != "" && t.Time != "" {
		if ts, err := time.ParseInLocation("2006-01-02 15:04", t.Date+" "+t.Time, loc); err == nil {
			return ts, true, true
		}
	}
	return time.Time{}, false, false
}

// openSubtaskCount is how many of a ticket row's subtasks are still open.
func (a App) openSubtaskCount(t model.Task) int {
	if !t.IsTicket() || a.idx == nil {
		return 0
	}
	tk, ok := a.idx.Ticket(t.TicketKey)
	if !ok {
		return 0
	}
	open := 0
	for _, c := range tk.Checkboxes {
		if !c.Done {
			open++
		}
	}
	return open
}

// calRange is the window the current scale covers, anchored on the cursor.
func (a App) calRange() (from, to time.Time) {
	loc := a.loc
	if loc == nil {
		loc = time.Local
	}
	anchor := a.calCursor
	if anchor.IsZero() {
		anchor = a.now()
	}
	anchor = startOfDay(anchor.In(loc))

	switch a.calScale {
	case calWeek:
		// Weeks start on Monday, which is what a working calendar means by a
		// week even though Go numbers Sunday as zero.
		offset := (int(anchor.Weekday()) + 6) % 7
		from = anchor.AddDate(0, 0, -offset)
		return from, from.AddDate(0, 0, 7)
	case calMonth:
		from = time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, loc)
		return from, from.AddDate(0, 1, 0)
	default:
		from = time.Date(anchor.Year(), time.January, 1, 0, 0, 0, 0, loc)
		return from, from.AddDate(1, 0, 0)
	}
}

// byDay buckets entries into calendar days.
func byDay(entries []calEntry) map[string][]calEntry {
	out := map[string][]calEntry{}
	for _, e := range entries {
		key := e.at.Format("2006-01-02")
		out[key] = append(out[key], e)
	}
	return out
}

// startOfDay is midnight on the day t falls in, in t's own location.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
