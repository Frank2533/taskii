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

// parseEvent reads an event out of one line of text.
//
// The syntax follows the app's existing habit of putting modifiers inline —
// a trailing clock time already turns a task into an appointment — so there is
// one thing to learn rather than a form to fill in:
//
//	standup 09:30-09:45 !tmr weekly
//	review 14:00-15:30 !fri
//	planning 10:00-11:00 monthly x2
func parseEvent(text string, now time.Time) (model.Event, bool) {
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

	return model.Event{
		ID:       strconv.FormatInt(now.UnixNano(), 36),
		Title:    name,
		Start:    startAt,
		End:      endAt,
		Repeat:   repeat,
		Interval: interval,
	}, true
}

// beginAddEvent opens the event input.
func (a App) beginAddEvent() (tea.Model, tea.Cmd) {
	a.mode = modeAddEvent
	a.input.SetValue("")
	a.input.Placeholder = "standup 09:30-09:45 !tmr weekly"
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
			a.err = "an event needs a time range, e.g. 09:30-10:00"
			return a, nil
		}
		a.events = append(a.events, ev)
		if !a.noPersist {
			_ = model.SaveEvents(a.events)
		}
		a.status = "added " + ev.Title
		return a, exportICS(a.vaultPath, a.icsOut, a.tasks, a.events, a.loc)
	}
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return a, cmd
}
