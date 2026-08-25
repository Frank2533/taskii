package model

import (
	"encoding/json"
	"os"
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

// maxOccurrences bounds expansion so a daily event with no end date cannot
// spin forever when asked for an implausible range.
const maxOccurrences = 2000

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
		if e.Start.Before(to) && end.After(from) {
			return []Occurrence{{Event: e, Start: e.Start, End: end}}
		}
		return nil
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
		end := start.Add(dur)
		if end.After(from) {
			out = append(out, Occurrence{Event: e, Start: start, End: end})
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
