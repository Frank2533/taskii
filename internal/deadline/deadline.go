// Package deadline parses and describes when work is due.
//
// taskii's original model has no deadlines at all: a task's Date is the day it
// belongs to, and "overdue" means it was carried over from an earlier day.
// That day bucket cannot double as a deadline, because it decides which day a
// task is listed under — giving a task a due date three days out would remove
// it from Today until that day arrived. So a deadline is a separate thing,
// expressed here.
package deadline

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Spec is a parsed deadline and start reminder.
type Spec struct {
	Due    time.Time
	HasDue bool

	// RemindAt is when to nudge the user to START, as opposed to when the
	// work is due. It is an absolute time, resolved from an offset at the
	// moment it was entered.
	RemindAt  time.Time
	HasRemind bool
}

// Empty reports whether nothing was specified.
func (s Spec) Empty() bool { return !s.HasDue && !s.HasRemind }

// EndOfDay is the instant a day's work is due by. "Due today" means due by the
// end of today, not by this time of day.
func EndOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 23, 59, 59, 0, t.Location())
}

// InDays is the deadline N days from now: 0 is today, 1 is tomorrow.
func InDays(now time.Time, n int) time.Time {
	return EndOfDay(now.AddDate(0, 0, n))
}

// EndOfWeek is the end of the coming Sunday, or today if it is already Sunday.
func EndOfWeek(now time.Time) time.Time {
	daysLeft := (7 - int(now.Weekday())) % 7
	return InDays(now, daysLeft)
}

// NextWeekday is the end of the next occurrence of the given weekday. Naming
// today's weekday means today, not a week from today.
func NextWeekday(now time.Time, want time.Weekday) time.Time {
	delta := (int(want) - int(now.Weekday()) + 7) % 7
	return InDays(now, delta)
}

var (
	// !2d, !3day, !10days
	daysRe = regexp.MustCompile(`^!(\d+)d(?:ay)?s?$`)
	// @3h, @90m, @2hr
	remindRe = regexp.MustCompile(`^@(\d+)(h|hr|hrs|hour|hours|m|min|mins|minute|minutes)$`)
)

var weekdays = map[string]time.Weekday{
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tues": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "weds": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
	"sun": time.Sunday, "sunday": time.Sunday,
}

// Parse pulls deadline tokens out of text and returns the text with them
// removed, so what the user typed as a modifier never ends up in the title.
//
// now must already be in the user's configured zone; every result is relative
// to it. A reminder offset resolves against now — the moment it was entered —
// which is what "@3h" means when you type it.
func Parse(text string, now time.Time) (string, Spec) {
	var spec Spec
	fields := strings.Fields(text)
	kept := make([]string, 0, len(fields))

	for _, f := range fields {
		lower := strings.ToLower(f)

		if due, ok := parseDueToken(lower, now); ok {
			spec.Due, spec.HasDue = due, true
			continue
		}
		if d, ok := parseRemindToken(lower); ok {
			spec.RemindAt, spec.HasRemind = now.Add(d), true
			continue
		}
		kept = append(kept, f)
	}
	return strings.Join(kept, " "), spec
}

func parseDueToken(tok string, now time.Time) (time.Time, bool) {
	if !strings.HasPrefix(tok, "!") {
		return time.Time{}, false
	}
	word := tok[1:]
	switch word {
	case "today", "tod", "0d":
		return InDays(now, 0), true
	case "tmr", "tom", "tomorrow":
		return InDays(now, 1), true
	case "w", "week", "eow":
		return EndOfWeek(now), true
	}
	// An explicit date, which is what a stored deadline renders back to.
	if d, err := time.ParseInLocation("2006-01-02", word, now.Location()); err == nil {
		return EndOfDay(time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, now.Location())), true
	}
	if m := daysRe.FindStringSubmatch(tok); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return time.Time{}, false
		}
		return InDays(now, n), true
	}
	if wd, ok := weekdays[word]; ok {
		return NextWeekday(now, wd), true
	}
	return time.Time{}, false
}

func parseRemindToken(tok string) (time.Duration, bool) {
	m := remindRe.FindStringSubmatch(tok)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, false
	}
	switch m[2][0] {
	case 'h':
		return time.Duration(n) * time.Hour, true
	default:
		return time.Duration(n) * time.Minute, true
	}
}

// Describe renders a deadline the way it was described to us: a whole-day
// promise, not a clock time.
func Describe(due time.Time, now time.Time) string {
	days := daysBetween(now, due)
	switch {
	case days < 0:
		if days == -1 {
			return "was due yesterday"
		}
		return fmt.Sprintf("was due %d days ago", -days)
	case days == 0:
		return "should be completed today"
	case days == 1:
		return "should be completed by tomorrow, end of day"
	case days < 7:
		return fmt.Sprintf("should be completed by %s, end of day", due.Format("Monday"))
	default:
		return fmt.Sprintf("should be completed by %s", due.Format("Mon 02 Jan"))
	}
}

// Short is a compact form for list rows.
func Short(due time.Time, now time.Time) string {
	days := daysBetween(now, due)
	switch {
	case days < 0:
		return fmt.Sprintf("%dd late", -days)
	case days == 0:
		return "today"
	case days == 1:
		return "tomorrow"
	case days < 7:
		return due.Format("Mon")
	default:
		return due.Format("02 Jan")
	}
}

// daysBetween counts calendar days from now's day to due's day, so "tomorrow"
// is 1 regardless of the clock time on either side.
func daysBetween(now, due time.Time) int {
	a := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	b := time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, due.Location())
	return int(b.Sub(a).Hours() / 24)
}

// Overdue reports whether a deadline has passed.
func Overdue(due, now time.Time) bool { return now.After(due) }

// Help lists the inline tokens, for the help bar and the picker.
func Help() string {
	return "!today !tmr !2d !fri !w set a deadline; @3h @90m set a start reminder"
}
