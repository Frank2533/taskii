package model

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"time"
)

const eventsFile = "events.json"

// Repeat is how often an event recurs.
type Repeat string

const (
	RepeatNone    Repeat = ""
	RepeatDaily   Repeat = "daily"
	RepeatWeekly  Repeat = "weekly"
	RepeatMonthly Repeat = "monthly"
	RepeatYearly  Repeat = "yearly"
)

// Repeats lists the cycle order used by the UI.
var Repeats = []Repeat{RepeatNone, RepeatDaily, RepeatWeekly, RepeatMonthly, RepeatYearly}

// Weekdays is Monday to Friday, the working week.
var Weekdays = []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}

// byDayCodes are the RFC 5545 two-letter weekday names.
var byDayCodes = map[time.Weekday]string{
	time.Sunday: "SU", time.Monday: "MO", time.Tuesday: "TU", time.Wednesday: "WE",
	time.Thursday: "TH", time.Friday: "FR", time.Saturday: "SA",
}

func (r Repeat) String() string {
	if r == RepeatNone {
		return "does not repeat"
	}
	return string(r)
}

// RRule renders the repeat as an RFC 5545 recurrence rule, empty when the
// event does not repeat.
func (e Event) RRule() string {
	if e.Repeat == RepeatNone {
		return ""
	}
	freq := map[Repeat]string{
		RepeatDaily:   "DAILY",
		RepeatWeekly:  "WEEKLY",
		RepeatMonthly: "MONTHLY",
		RepeatYearly:  "YEARLY",
	}[e.Repeat]
	if freq == "" {
		return ""
	}
	rule := "FREQ=" + freq
	if len(e.Days) > 0 && e.Repeat == RepeatWeekly {
		codes := make([]string, 0, len(e.Days))
		for _, d := range SortWeekdays(e.Days) {
			codes = append(codes, byDayCodes[d])
		}
		rule += ";BYDAY=" + strings.Join(codes, ",")
	}
	if e.Interval > 1 {
		rule += ";INTERVAL=" + itoa(e.Interval)
	}
	if e.Until != nil {
		rule += ";UNTIL=" + e.Until.UTC().Format("20060102T150405Z")
	}
	return rule
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// Event is an appointment with a start and an end, optionally recurring.
//
// It is distinct from an appointment-kind Task: a task is work to finish and
// can be ticked off, an event is time that is spoken for whether or not
// anything is done in it.
type Event struct {
	ID     string    `json:"id"`
	Title  string    `json:"title"`
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	AllDay bool      `json:"all_day,omitempty"`

	Repeat Repeat `json:"repeat,omitempty"`
	// Interval repeats every N periods; 0 and 1 both mean every period.
	Interval int `json:"interval,omitempty"`
	// Until bounds a repeat. Nil repeats indefinitely.
	Until *time.Time `json:"until,omitempty"`

	// Except lists occurrence start times removed from the series, which is
	// how a single occurrence is changed or cancelled without disturbing the
	// rest of it.
	Except []time.Time `json:"except,omitempty"`

	// Days restricts a weekly repeat to particular weekdays, which is how a
	// standup that runs Monday to Friday is expressed. Empty means the repeat
	// falls on whatever weekday Start does.
	Days []time.Weekday `json:"days,omitempty"`

	Location string `json:"location,omitempty"`
	Notes    string `json:"notes,omitempty"`
}

// Duration is how long one occurrence lasts.
func (e Event) Duration() time.Duration {
	if e.End.IsZero() || !e.End.After(e.Start) {
		return time.Hour
	}
	return e.End.Sub(e.Start)
}

// Occurrence is one instance of an event on the calendar.
type Occurrence struct {
	Event Event
	Start time.Time
	End   time.Time
}

// step advances one repeat period.
//
// Monthly and yearly steps use AddDate, which normalises overflow: the 31st in
// a 30-day month lands on the 1st of the next. That is Go's documented
// behaviour and it is left alone rather than clamped, so an occurrence is
// never silently dropped from a month.
func (e Event) step(t time.Time, n int) time.Time {
	interval := e.Interval
	if interval < 1 {
		interval = 1
	}
	k := interval * n
	switch e.Repeat {
	case RepeatDaily:
		return t.AddDate(0, 0, k)
	case RepeatWeekly:
		return t.AddDate(0, 0, 7*k)
	case RepeatMonthly:
		return t.AddDate(0, k, 0)
	case RepeatYearly:
		return t.AddDate(k, 0, 0)
	default:
		return t
	}
}

// SortWeekdays orders a day set Monday first, so a weekly repeat is expanded
// and rendered in the order a working week is read.
func SortWeekdays(days []time.Weekday) []time.Weekday {
	out := make([]time.Weekday, len(days))
	copy(out, days)
	sort.Slice(out, func(i, j int) bool {
		return mondayIndex(out[i]) < mondayIndex(out[j])
	})
	return out
}

// mondayIndex numbers weekdays from Monday, since Go numbers them from Sunday.
func mondayIndex(d time.Weekday) int { return (int(d) + 6) % 7 }

// maxOccurrences bounds expansion so a daily event with no end date cannot
// spin forever when asked for an implausible range.
const maxOccurrences = 2000

// excluded reports whether an occurrence has been removed from the series.
//
// Matching is to the minute rather than exact: a stored exception and a
// computed occurrence can differ in seconds or monotonic detail after a
// round trip through JSON, and an exception that silently stopped matching
// would resurrect an occurrence the user had already changed.
func (e Event) excluded(start time.Time) bool {
	for _, ex := range e.Except {
		if ex.UTC().Truncate(time.Minute).Equal(start.UTC().Truncate(time.Minute)) {
			return true
		}
	}
	return false
}

// ExceptDates renders the exclusions as an RFC 5545 EXDATE value, empty when
// there are none.
func (e Event) ExceptDates() string {
	if len(e.Except) == 0 {
		return ""
	}
	out := make([]string, 0, len(e.Except))
	for _, ex := range e.Except {
		out = append(out, ex.UTC().Format("20060102T150405Z"))
	}
	return strings.Join(out, ",")
}

// Occurrences expands an event into the instances overlapping [from, to).
func (e Event) Occurrences(from, to time.Time) []Occurrence {
	if !to.After(from) {
		return nil
	}
	dur := e.Duration()

	if e.Repeat == RepeatNone {
		end := e.End
		if end.IsZero() {
			end = e.Start.Add(dur)
		}
		if e.Start.Before(to) && end.After(from) && !e.excluded(e.Start) {
			return []Occurrence{{Event: e, Start: e.Start, End: end}}
		}
		return nil
	}

	if e.Repeat == RepeatWeekly && len(e.Days) > 0 {
		return e.weeklyByDay(from, to, dur)
	}

	var out []Occurrence
	for n := 0; n < maxOccurrences; n++ {
		start := e.step(e.Start, n)
		if !start.Before(to) {
			break
		}
		if e.Until != nil && start.After(*e.Until) {
			break
		}
		if e.excluded(start) {
			continue
		}
		end := start.Add(dur)
		if end.After(from) {
			out = append(out, Occurrence{Event: e, Start: start, End: end})
		}
	}
	return out
}

// weeklyByDay expands a weekly repeat that names its own weekdays.
//
// The event's own weekday is ignored: what matters is the time of day and the
// set of days chosen, so a standup created on a Wednesday and set to weekdays
// still runs on the Monday.
func (e Event) weeklyByDay(from, to time.Time, dur time.Duration) []Occurrence {
	interval := e.Interval
	if interval < 1 {
		interval = 1
	}
	days := SortWeekdays(e.Days)

	// Anchor on the Monday of the week the event starts in, so the interval
	// counts whole weeks rather than sliding with the start's weekday.
	loc := e.Start.Location()
	anchor := time.Date(e.Start.Year(), e.Start.Month(), e.Start.Day(), 0, 0, 0, 0, loc).
		AddDate(0, 0, -mondayIndex(e.Start.Weekday()))

	var out []Occurrence
	for w := 0; w < maxOccurrences; w++ {
		weekStart := anchor.AddDate(0, 0, 7*interval*w)
		if !weekStart.Before(to) {
			break
		}
		for _, d := range days {
			day := weekStart.AddDate(0, 0, mondayIndex(d))
			start := time.Date(day.Year(), day.Month(), day.Day(),
				e.Start.Hour(), e.Start.Minute(), 0, 0, loc)
			if start.Before(e.Start) {
				continue
			}
			if e.Until != nil && start.After(*e.Until) {
				return out
			}
			if !start.Before(to) || e.excluded(start) {
				continue
			}
			if end := start.Add(dur); end.After(from) {
				out = append(out, Occurrence{Event: e, Start: start, End: end})
			}
		}
		if len(out) >= maxOccurrences {
			break
		}
	}
	return out
}

// EventsOccurring expands a set of events over a range, in start order.
func EventsOccurring(events []Event, from, to time.Time) []Occurrence {
	var out []Occurrence
	for _, e := range events {
		out = append(out, e.Occurrences(from, to)...)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Start.Before(out[j-1].Start); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func LoadEvents() ([]Event, error) {
	b, err := readData(eventsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return nil, nil
	}
	var events []Event
	if err := json.Unmarshal(b, &events); err != nil {
		return nil, err
	}
	return events, nil
}

func SaveEvents(events []Event) error {
	b, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return err
	}
	return writeData(eventsFile, b)
}
