// Package remind decides which event reminders are due.
package remind

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"taskii/internal/model"
)

// Leads are how far ahead of an event to warn.
var Leads = []time.Duration{15 * time.Minute, 5 * time.Minute, 1 * time.Minute}

// grace is how late a reminder may still be delivered.
//
// A "15 minutes before" warning that arrives three minutes before is worse
// than none: it is wrong about the thing it exists to say. Reminders missed
// because the app was closed are recorded as fired and skipped rather than
// delivered in a burst at startup.
const grace = 90 * time.Second

// retention is how long a fired marker is kept before pruning.
const retention = 24 * time.Hour

const fileName = "reminders.json"

// Due is one reminder that should be delivered now.
type Due struct {
	Key        string
	Title      string
	Lead       time.Duration
	EventStart time.Time
}

// Message is the human wording for a reminder.
func (d Due) Message() string {
	mins := int(d.Lead / time.Minute)
	unit := "minutes"
	if mins == 1 {
		unit = "minute"
	}
	return fmt.Sprintf("%s in %d %s (%s)", d.Title, mins, unit, d.EventStart.Format("15:04"))
}

// Fired remembers which reminders have already gone out, keyed by occurrence
// and lead so a repeating event warns again on its next occurrence but never
// twice for the same one.
type Fired struct {
	Keys map[string]time.Time `json:"keys"`
}

func NewFired() *Fired { return &Fired{Keys: map[string]time.Time{}} }

func (f *Fired) has(key string) bool {
	_, ok := f.Keys[key]
	return ok
}

func (f *Fired) mark(key string, occurrence time.Time) {
	if f.Keys == nil {
		f.Keys = map[string]time.Time{}
	}
	f.Keys[key] = occurrence
}

// Prune drops markers for occurrences well in the past, so the file does not
// grow without bound for a daily event.
func (f *Fired) Prune(now time.Time) {
	for k, occ := range f.Keys {
		if now.Sub(occ) > retention {
			delete(f.Keys, k)
		}
	}
}

func path(dir string) string { return filepath.Join(dir, fileName) }

func Load(dir string) (*Fired, error) {
	f := NewFired()
	b, err := os.ReadFile(path(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return f, err
	}
	if len(b) == 0 {
		return f, nil
	}
	if err := json.Unmarshal(b, f); err != nil {
		return NewFired(), err
	}
	if f.Keys == nil {
		f.Keys = map[string]time.Time{}
	}
	return f, nil
}

func (f *Fired) Save(dir string) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path(dir), b, 0o644)
}

// key identifies one reminder: this event, this occurrence, this lead.
func key(e model.Event, occurrence time.Time, lead time.Duration) string {
	return fmt.Sprintf("%s|%s|%dm", e.ID, occurrence.UTC().Format(time.RFC3339), int(lead/time.Minute))
}

// Pending returns the reminders to deliver now, marking everything it considers so
// nothing fires twice.
//
// It mutates fired, including for reminders it decides are too late to send:
// leaving those unmarked would deliver them at the next sweep instead.
func Pending(events []model.Event, now time.Time, fired *Fired) []Due {
	if len(events) == 0 {
		return nil
	}
	maxLead := time.Duration(0)
	for _, l := range Leads {
		if l > maxLead {
			maxLead = l
		}
	}
	// Look a little either side: back far enough to catch an occurrence whose
	// reminders are still within grace, forward far enough to see the next.
	from := now.Add(-maxLead)
	to := now.Add(maxLead + time.Minute)

	var out []Due
	for _, o := range model.EventsOccurring(events, from, to) {
		for _, lead := range Leads {
			at := o.Start.Add(-lead)
			k := key(o.Event, o.Start, lead)
			if fired.has(k) || now.Before(at) {
				continue
			}
			fired.mark(k, o.Start)
			if now.Sub(at) > grace {
				continue
			}
			out = append(out, Due{
				Key: k, Title: o.Event.Title, Lead: lead, EventStart: o.Start,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Lead > out[j].Lead })
	fired.Prune(now)
	return out
}
