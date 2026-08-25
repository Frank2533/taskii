package ui

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"taskii/internal/deadline"
	"taskii/internal/model"
)

// timeRangeRe matches "09:30-10:45", the start and end of an event.
var timeRangeRe = regexp.MustCompile(`^(\d{1,2}):(\d{2})-(\d{1,2}):(\d{2})$`)

// intervalRe matches "x2", repeating every other period.
var intervalRe = regexp.MustCompile(`^x(\d+)$`)

var repeatWords = map[string]model.Repeat{
	"daily": model.RepeatDaily, "weekly": model.RepeatWeekly,
	"monthly": model.RepeatMonthly, "yearly": model.RepeatYearly,
	"annually": model.RepeatYearly,
}

// dayWords name a single weekday for a day set.
var dayWords = map[string]time.Weekday{
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tues": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "weds": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
	"sun": time.Sunday, "sunday": time.Sunday,
}

// parseDaySet reads "mon,wed,fri" or "weekdays" into a set of weekdays.
//
// It is checked before the deadline tokens, which also understand weekday
// names: there, "!fri" picks the next Friday, whereas here a bare "fri" names
// a day the event recurs on. The "!" is what tells them apart.
func parseDaySet(tok string) ([]time.Weekday, bool) {
	switch tok {
	case "weekdays", "weekday", "mon-fri", "workdays":
		return model.Weekdays, true
	case "weekends", "weekend":
		return []time.Weekday{time.Saturday, time.Sunday}, true
	}
	if !strings.Contains(tok, ",") {
		if d, ok := dayWords[tok]; ok {
			return []time.Weekday{d}, true
		}
		return nil, false
	}
	var out []time.Weekday
	for _, part := range strings.Split(tok, ",") {
		d, ok := dayWords[strings.TrimSpace(part)]
		if !ok {
			return nil, false
		}
		out = append(out, d)
	}
	return out, len(out) > 0
}

// parseEvent reads an event out of one line of text.
//
// The syntax follows the app's existing habit of putting modifiers inline —
// a trailing clock time already turns a task into an appointment — so there is
// one thing to learn rather than a form to fill in:
//
//	standup 09:30-09:45 weekdays
//	gym 07:00-08:00 mon,wed,fri
//	review 14:00-15:30 !fri
//	planning 10:00-11:00 monthly x2
func parseEvent(text string, now time.Time) (model.Event, bool) {
	// A day set is pulled out first, because deadline parsing also claims
	// weekday names and would swallow "fri" as "the coming Friday".
	var days []time.Weekday
	var kept []string
	for _, f := range strings.Fields(text) {
		if d, ok := parseDaySet(strings.ToLower(f)); ok && days == nil {
			days = d
			continue
		}
		kept = append(kept, f)
	}
	text = strings.Join(kept, " ")

	// The day comes from the same tokens deadlines use.
	rest, spec := deadline.Parse(text, now)

	day := now
	if spec.HasDue {
		day = spec.Due
	}

	var (
		title    []string
		start    = -1
		end      = -1
		repeat   = model.RepeatNone
		interval = 0
	)
	for _, f := range strings.Fields(rest) {
		lower := strings.ToLower(f)
		if m := timeRangeRe.FindStringSubmatch(lower); m != nil && start < 0 {
			sh, _ := strconv.Atoi(m[1])
			sm, _ := strconv.Atoi(m[2])
			eh, _ := strconv.Atoi(m[3])
			em, _ := strconv.Atoi(m[4])
			if sh < 24 && eh < 24 && sm < 60 && em < 60 {
				start, end = sh*60+sm, eh*60+em
				continue
			}
		}
		if r, ok := repeatWords[lower]; ok && repeat == model.RepeatNone {
			repeat = r
			continue
		}
		if m := intervalRe.FindStringSubmatch(lower); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 1 {
				interval = n
				continue
			}
		}
		title = append(title, f)
	}

	// Naming days implies a weekly repeat: "standup 09:30-09:45 weekdays"
	// should not need the word "weekly" as well.
	if len(days) > 0 && repeat == model.RepeatNone {
		repeat = model.RepeatWeekly
	}

	name := strings.TrimSpace(strings.Join(title, " "))
	if name == "" || start < 0 {
		// Without a time range this is a task, not an event; refusing here
		// keeps the two from blurring together.
		return model.Event{}, false
	}

	y, mo, d := day.Date()
	loc := day.Location()
	startAt := time.Date(y, mo, d, start/60, start%60, 0, 0, loc)
	endAt := time.Date(y, mo, d, end/60, end%60, 0, 0, loc)
	if !endAt.After(startAt) {
		// An end before the start means it runs past midnight.
		endAt = endAt.AddDate(0, 0, 1)
	}

	if repeat != model.RepeatWeekly {
		// Day sets only mean anything within a week.
		days = nil
	}

	return model.Event{
		ID:       strconv.FormatInt(now.UnixNano(), 36),
		Title:    name,
		Start:    startAt,
		End:      endAt,
		Repeat:   repeat,
		Interval: interval,
		Days:     days,
	}, true
}

// dayCodes name a weekday for the day-set syntax, Monday first.
var dayCodes = map[time.Weekday]string{
	time.Monday: "mon", time.Tuesday: "tue", time.Wednesday: "wed",
	time.Thursday: "thu", time.Friday: "fri", time.Saturday: "sat", time.Sunday: "sun",
}

// formatEvent renders an event back into the syntax it was typed in.
//
// Editing reuses the same one-line grammar rather than a separate form: what
// is shown for editing is exactly what would have created the event, so there
// is nothing extra to learn and no second representation to keep in step.
func formatEvent(e model.Event, now time.Time) string {
	parts := []string{e.Title, e.Start.Format("15:04") + "-" + e.End.Format("15:04")}

	// The date is only worth stating when it is not today, and a repeating
	// event's date is implied by its rule.
	if e.Repeat == model.RepeatNone && !sameDay(e.Start, now) {
		parts = append(parts, "!"+e.Start.Format("2006-01-02"))
	}

	if len(e.Days) > 0 {
		if isWeekdaySet(e.Days) {
			parts = append(parts, "weekdays")
		} else {
			var names []string
			for _, d := range model.SortWeekdays(e.Days) {
				names = append(names, dayCodes[d])
			}
			parts = append(parts, strings.Join(names, ","))
		}
	} else if e.Repeat != model.RepeatNone {
		parts = append(parts, string(e.Repeat))
	}

	if e.Interval > 1 {
		parts = append(parts, "x"+strconv.Itoa(e.Interval))
	}
	return strings.Join(parts, " ")
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func isWeekdaySet(days []time.Weekday) bool {
	if len(days) != len(model.Weekdays) {
		return false
	}
	seen := map[time.Weekday]bool{}
	for _, d := range days {
		seen[d] = true
	}
	for _, d := range model.Weekdays {
		if !seen[d] {
			return false
		}
	}
	return true
}

// beginEditEvent opens the input primed with the selected event.
func (a App) beginEditEvent() (tea.Model, tea.Cmd) {
	ev, ok := a.selectedEvent()
	if !ok {
		if _, onSomething := a.selectedEntry(); onSomething {
			a.setErr("only events are edited here — tasks and tickets are edited where they live")
		} else {
			a.setErr("no event selected")
		}
		return a, nil
	}
	a.editingEvent = ev.ID
	a.mode = modeEditEvent
	a.input.SetValue(formatEvent(ev, a.now()))
	a.input.Placeholder = "edit, then enter"
	a.input.CursorEnd()
	a.input.Focus()
	return a, nil
}

// updateEditEvent drives the edit input.
func (a App) updateEditEvent(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.mode = modeNormal
		a.editingEvent = ""
		a.input.Blur()
		a.input.SetValue("")
		return a, nil
	case "enter":
		text := a.input.Value()
		id := a.editingEvent
		a.mode = modeNormal
		a.editingEvent = ""
		a.input.Blur()
		a.input.SetValue("")

		parsed, ok := parseEvent(text, a.now())
		if !ok {
			a.setErr("an event needs a time range, e.g. 09:30-10:00")
			return a, nil
		}
		original, ok := a.eventByID(id)
		if !ok {
			a.setErr("that event no longer exists")
			return a, nil
		}
		if original.Repeat != model.RepeatNone {
			// Which occurrences a change reaches is genuinely ambiguous for a
			// repeat, and guessing rewrites a series that cannot easily be
			// restored.
			_, spec := deadline.Parse(text, a.now())
			return a.beginScopePrompt(scopeEdit, original, a.scopeOccurrence(original), parsed, spec.HasDue)
		}
		for i := range a.events {
			if a.events[i].ID != id {
				continue
			}
			// The identity is kept so the event stays the same entry in a
			// subscriber's calendar rather than arriving as a new one, and so
			// reminders already sent for it are still recognised as sent.
			parsed.ID = id
			a.events[i] = parsed
			if !a.noPersist {
				_ = model.SaveEvents(a.events)
			}
			a.setStatus("updated " + parsed.Title)
			return a, exportICS(a.vaultPath, a.icsOut, a.tasks, a.events, a.loc)
		}
		a.setErr("that event no longer exists")
		return a, nil
	}
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return a, cmd
}

// scopeOccurrence is the occurrence a change was made from: the entry under
// the cursor, falling back to the series start when the cursor is elsewhere.
func (a App) scopeOccurrence(ev model.Event) time.Time {
	if e, ok := a.selectedEntry(); ok && e.eventID == ev.ID {
		return e.at
	}
	return ev.Start
}

// deleteSelectedEvent removes the event under the cursor.
func (a App) deleteSelectedEvent() (tea.Model, tea.Cmd) {
	ev, ok := a.selectedEvent()
	if !ok {
		a.setErr("no event selected")
		return a, nil
	}
	if ev.Repeat != model.RepeatNone {
		return a.beginScopePrompt(scopeDelete, ev, a.scopeOccurrence(ev), model.Event{}, false)
	}
	out := a.events[:0:0]
	for _, e := range a.events {
		if e.ID != ev.ID {
			out = append(out, e)
		}
	}
	a.events = out
	if !a.noPersist {
		_ = model.SaveEvents(a.events)
	}
	a.setStatus("deleted " + ev.Title)
	return a, exportICS(a.vaultPath, a.icsOut, a.tasks, a.events, a.loc)
}

// beginAddEvent opens the event input.
func (a App) beginAddEvent() (tea.Model, tea.Cmd) {
	a.mode = modeAddEvent
	a.input.SetValue("")
	a.input.Placeholder = "standup 09:30-09:45 weekdays"
	a.input.Focus()
	return a, nil
}

// updateAddEvent drives the event input.
func (a App) updateAddEvent(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.mode = modeNormal
		a.input.Blur()
		a.input.SetValue("")
		return a, nil
	case "enter":
		text := a.input.Value()
		a.mode = modeNormal
		a.input.Blur()
		a.input.SetValue("")

		ev, ok := parseEvent(text, a.now())
		if !ok {
			a.setErr("an event needs a time range, e.g. 09:30-10:00")
			return a, nil
		}
		a.events = append(a.events, ev)
		if !a.noPersist {
			_ = model.SaveEvents(a.events)
		}
		a.setStatus("added " + ev.Title)
		return a, exportICS(a.vaultPath, a.icsOut, a.tasks, a.events, a.loc)
	}
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return a, cmd
}
