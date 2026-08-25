// Package export turns indexed vault content and local tasks into a calendar.
package export

import (
	"strings"
	"time"

	"taskii/internal/ics"
	"taskii/internal/model"
	"taskii/internal/para"
)

// Calendar builds the .ics model from a vault index plus taskii's own timed
// entries.
//
// DTSTAMP comes from each source note's updated time rather than the clock, so
// re-exporting an unchanged vault produces an identical file — see the note on
// determinism in package ics.
func Calendar(name string, idx *para.Index, tasks []model.Task, events []model.Event, loc *time.Location) *ics.Calendar {
	if loc == nil {
		loc = time.Local
	}
	cal := &ics.Calendar{Name: name, Loc: loc}
	if idx != nil {
		for _, t := range idx.Tickets {
			if !t.HasDue || t.Closed() {
				continue
			}
			cal.Events = append(cal.Events, ics.Event{
				UID:         ics.UID("ticket", t.Key),
				Summary:     t.Title(),
				Start:       t.Due,
				AllDay:      true,
				Description: describeTicket(t),
				URL:         t.Link,
				Categories:  categories("Ticket", t.Area),
				Stamp:       t.Updated,
			})
		}
		for _, p := range idx.Projects {
			if !p.HasTarget {
				continue
			}
			cal.Events = append(cal.Events, ics.Event{
				UID:        ics.UID("project", p.Path),
				Summary:    "Target: " + p.Title,
				Start:      p.Target,
				AllDay:     true,
				Categories: categories("Project", p.Area),
			})
		}
	}
	for _, e := range events {
		start := e.Start.In(loc)
		end := e.End
		if end.IsZero() {
			end = start.Add(e.Duration())
		}
		cal.Events = append(cal.Events, ics.Event{
			UID:         ics.UID("event", e.ID),
			Summary:     e.Title,
			Start:       start,
			End:         end.In(loc),
			AllDay:      e.AllDay,
			Description: e.Notes,
			Categories:  categories("Event", ""),
			RRule:       e.RRule(),
			ExDate:      e.ExceptDates(),
			Stamp:       e.Start,
		})
	}
	for _, t := range tasks {
		if !t.IsAppointment() || t.Done {
			continue
		}
		start, ok := appointmentStart(t, loc)
		if !ok {
			continue
		}
		cal.Events = append(cal.Events, ics.Event{
			UID:        ics.UID("appt", t.ID),
			Summary:    t.Title,
			Start:      start,
			Categories: categories("Appointment", ""),
			Stamp:      t.CreatedAt,
		})
	}
	return cal
}

// appointmentStart combines a task's date and time in the configured zone. A
// stored appointment carries wall-clock text with no zone of its own, so it is
// interpreted where the user is, not where the machine thinks it is.
func appointmentStart(t model.Task, loc *time.Location) (time.Time, bool) {
	if t.Date == "" || t.Time == "" {
		return time.Time{}, false
	}
	ts, err := time.ParseInLocation("2006-01-02 15:04", t.Date+" "+t.Time, loc)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func describeTicket(t para.Ticket) string {
	var parts []string
	if t.Status != "" {
		parts = append(parts, "Status: "+t.Status)
	}
	if t.Priority != "" {
		parts = append(parts, "Priority: "+t.Priority)
	}
	if t.Assignee != "" {
		parts = append(parts, "Assignee: "+t.Assignee)
	}
	if t.Area != "" {
		parts = append(parts, "Area: "+t.Area)
	}
	return strings.Join(parts, "\n")
}

func categories(kind, area string) []string {
	out := []string{kind}
	if strings.TrimSpace(area) != "" {
		out = append(out, area)
	}
	return out
}
