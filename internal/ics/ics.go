// Package ics renders an RFC 5545 calendar.
//
// Output is deterministic on purpose. The calendar is written into a vault that
// is watched and committed on every save, so a file that differs on each run —
// a regenerated DTSTAMP, events in map order — produces an endless stream of
// no-op commits and, across two machines, merge conflicts. Given unchanged
// input this package produces byte-identical output, and WriteIfChanged skips
// the write entirely when nothing moved.
package ics

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ProdID identifies the generator, per RFC 5545 section 3.7.3.
const ProdID = "-//taskii//PARA vault calendar//EN"

// DefaultDuration is how long a timed entry lasts when the source only gives a
// start time.
const DefaultDuration = 30 * time.Minute

// Event is one VEVENT.
type Event struct {
	UID     string
	Summary string

	// Start is the moment the event begins, in any location; it is converted
	// on render.
	Start time.Time

	// End may be zero, in which case DefaultDuration applies to timed events.
	End time.Time

	// AllDay renders DTSTART;VALUE=DATE, which carries no time and therefore
	// no zone — the correct encoding for a due date, which is a calendar day
	// rather than an instant.
	AllDay bool

	Description string
	URL         string
	Categories  []string

	// RRule is an RFC 5545 recurrence rule without the "RRULE:" prefix,
	// empty for a one-off. A repeating event is emitted once with its rule
	// rather than expanded, so a subscriber's calendar keeps the series
	// intact instead of receiving hundreds of unrelated entries.
	RRule string

	// Stamp is DTSTAMP. Callers should pass something derived from the source
	// note (its updated time), never the current clock, or every export
	// rewrites the file.
	Stamp time.Time
}

// Calendar is a set of events plus the zone their local times refer to.
type Calendar struct {
	Name   string
	Loc    *time.Location
	Events []Event
}

// Render produces the .ics body.
//
// Timed events are emitted as UTC instants rather than TZID references into a
// VTIMEZONE block. A VTIMEZONE has to restate the zone's DST transition rules,
// and hand-rolling those from tzdata is a well-known source of off-by-an-hour
// bugs; a UTC instant is unambiguous, universally supported, and still derived
// from the user's configured zone. All-day events carry no time at all, so they
// need neither.
func (c *Calendar) Render() []byte {
	loc := c.Loc
	if loc == nil {
		loc = time.Local
	}

	events := make([]Event, len(c.Events))
	copy(events, c.Events)
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].Start.Equal(events[j].Start) {
			return events[i].Start.Before(events[j].Start)
		}
		if events[i].Summary != events[j].Summary {
			return events[i].Summary < events[j].Summary
		}
		return events[i].UID < events[j].UID
	})

	var b bytes.Buffer
	line := func(s string) { b.WriteString(fold(s)) }

	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:" + ProdID)
	line("CALSCALE:GREGORIAN")
	line("METHOD:PUBLISH")
	if c.Name != "" {
		line("X-WR-CALNAME:" + escape(c.Name))
	}
	// A display hint for clients that honour it; it does not affect the UTC
	// instants above.
	line("X-WR-TIMEZONE:" + loc.String())

	for _, e := range events {
		line("BEGIN:VEVENT")
		line("UID:" + escape(e.UID))
		line("DTSTAMP:" + utcStamp(e.Stamp))
		if e.AllDay {
			day := e.Start.In(loc)
			line("DTSTART;VALUE=DATE:" + day.Format("20060102"))
			// DTEND is exclusive for date values, so a one-day event ends the
			// following day. Omitting it makes some clients render nothing.
			end := e.End
			if end.IsZero() {
				end = day.AddDate(0, 0, 1)
			}
			line("DTEND;VALUE=DATE:" + end.In(loc).Format("20060102"))
		} else {
			start := e.Start.In(loc)
			end := e.End
			if end.IsZero() {
				end = start.Add(DefaultDuration)
			}
			line("DTSTART:" + utcStamp(start))
			line("DTEND:" + utcStamp(end))
		}
		if e.RRule != "" {
			line("RRULE:" + e.RRule)
		}
		line("SUMMARY:" + escape(e.Summary))
		if e.Description != "" {
			line("DESCRIPTION:" + escape(e.Description))
		}
		if e.URL != "" {
			line("URL:" + escape(e.URL))
		}
		if len(e.Categories) > 0 {
			cats := make([]string, 0, len(e.Categories))
			for _, c := range e.Categories {
				cats = append(cats, escape(c))
			}
			line("CATEGORIES:" + strings.Join(cats, ","))
		}
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return b.Bytes()
}

// epoch is the DTSTAMP fallback. A zero Stamp must still render a valid,
// stable value rather than the current time.
var epoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

func utcStamp(t time.Time) string {
	if t.IsZero() {
		t = epoch
	}
	return t.UTC().Format("20060102T150405Z")
}

// escape applies RFC 5545 section 3.3.11 text escaping.
func escape(s string) string {
	r := strings.NewReplacer(
		"\\", "\\\\",
		";", "\\;",
		",", "\\,",
		"\r\n", "\\n",
		"\n", "\\n",
		"\r", "\\n",
	)
	return r.Replace(s)
}

// fold wraps a content line to 75 octets and terminates it with CRLF, per
// section 3.1. Continuation lines begin with a single space.
func fold(s string) string {
	const limit = 75
	var b strings.Builder
	count := 0
	for _, r := range s {
		n := len(string(r))
		if count+n > limit {
			b.WriteString("\r\n ")
			count = 1
		}
		b.WriteRune(r)
		count += n
	}
	b.WriteString("\r\n")
	return b.String()
}

// WriteIfChanged writes data to path only when the content differs, and
// reports whether it wrote. Skipping identical writes is what keeps the
// vault's file watcher from committing an unchanged calendar.
func WriteIfChanged(path string, data []byte) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil && sha256.Sum256(existing) == sha256.Sum256(data) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	tmp, err := os.CreateTemp(dir, ".taskii-ics-*.tmp")
	if err != nil {
		return false, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return false, err
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return false, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return false, err
	}
	return true, nil
}

// UID builds a stable identifier. Deriving it from the source key means a
// re-export updates the existing entry in a subscriber's calendar instead of
// creating a duplicate.
func UID(kind, key string) string {
	if key == "" {
		key = "unknown"
	}
	return fmt.Sprintf("%s-%s@taskii", strings.ToLower(kind), strings.ToLower(key))
}
