package ui

import (
	"testing"
	"time"

	"taskii/internal/model"
)

func TestParseEventReadsTimeDayAndRepeat(t *testing.T) {
	now := time.Date(2026, 8, 26, 15, 0, 0, 0, time.UTC) // a Wednesday
	ev, ok := parseEvent("standup 09:30-09:45 !tmr weekly", now)
	if !ok {
		t.Fatal("not parsed")
	}
	if ev.Title != "standup" {
		t.Errorf("title = %q, want the tokens stripped", ev.Title)
	}
	if h, m := ev.Start.Hour(), ev.Start.Minute(); h != 9 || m != 30 {
		t.Errorf("start = %v", ev.Start)
	}
	if ev.End.Sub(ev.Start) != 15*time.Minute {
		t.Errorf("duration = %v, want 15m", ev.End.Sub(ev.Start))
	}
	if ev.Start.Day() != 27 {
		t.Errorf("day = %d, want tomorrow (27)", ev.Start.Day())
	}
	if ev.Repeat != model.RepeatWeekly {
		t.Errorf("repeat = %q", ev.Repeat)
	}
}

func TestParseEventDefaultsToToday(t *testing.T) {
	now := time.Date(2026, 8, 26, 15, 0, 0, 0, time.UTC)
	ev, ok := parseEvent("review 14:00-15:30", now)
	if !ok {
		t.Fatal("not parsed")
	}
	if ev.Start.Day() != 26 {
		t.Errorf("day = %d, want today", ev.Start.Day())
	}
	if ev.Repeat != model.RepeatNone {
		t.Errorf("repeat = %q, want none", ev.Repeat)
	}
}

func TestParseEventInterval(t *testing.T) {
	now := time.Date(2026, 8, 26, 15, 0, 0, 0, time.UTC)
	ev, ok := parseEvent("planning 10:00-11:00 monthly x2", now)
	if !ok {
		t.Fatal("not parsed")
	}
	if ev.Interval != 2 {
		t.Errorf("interval = %d, want 2", ev.Interval)
	}
	if ev.Title != "planning" {
		t.Errorf("title = %q", ev.Title)
	}
}

// Without a time range it is a task, not an event; blurring the two would put
// work with no duration on the calendar as if it occupied time.
func TestParseEventRequiresATimeRange(t *testing.T) {
	now := time.Date(2026, 8, 26, 15, 0, 0, 0, time.UTC)
	if _, ok := parseEvent("just a thought !tmr", now); ok {
		t.Error("parsed an event with no time range")
	}
}

// An end before the start runs past midnight rather than being negative.
func TestParseEventWrapsPastMidnight(t *testing.T) {
	now := time.Date(2026, 8, 26, 15, 0, 0, 0, time.UTC)
	ev, ok := parseEvent("deploy 23:00-01:00", now)
	if !ok {
		t.Fatal("not parsed")
	}
	if !ev.End.After(ev.Start) {
		t.Errorf("end %v is not after start %v", ev.End, ev.Start)
	}
	if ev.End.Sub(ev.Start) != 2*time.Hour {
		t.Errorf("duration = %v, want 2h", ev.End.Sub(ev.Start))
	}
}

// A repeating event is exported once with its rule, so a subscriber keeps the
// series rather than receiving hundreds of unrelated entries.
func TestRepeatingEventExportsAsOneEntryWithARule(t *testing.T) {
	a, _ := dashApp(t)
	a = press(t, a, "3")
	a = press(t, a, "a")
	a.input.SetValue("standup 09:30-09:45 weekly")
	a = press(t, a, "enter")
	if a.err != "" {
		t.Fatalf("add reported: %s", a.err)
	}
	if len(a.events) != 1 {
		t.Fatalf("events = %d, want 1", len(a.events))
	}
	if a.events[0].RRule() != "FREQ=WEEKLY" {
		t.Errorf("rule = %q", a.events[0].RRule())
	}
}

func TestParseEventWeekdays(t *testing.T) {
	now := time.Date(2026, 8, 26, 15, 0, 0, 0, time.UTC) // Wednesday
	ev, ok := parseEvent("standup 09:30-09:45 weekdays", now)
	if !ok {
		t.Fatal("not parsed")
	}
	if ev.Title != "standup" {
		t.Errorf("title = %q", ev.Title)
	}
	// Naming days implies weekly; the word "weekly" should not be required.
	if ev.Repeat != model.RepeatWeekly {
		t.Errorf("repeat = %q, want weekly", ev.Repeat)
	}
	if len(ev.Days) != 5 {
		t.Errorf("days = %v, want Mon-Fri", ev.Days)
	}
	if ev.RRule() != "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR" {
		t.Errorf("rule = %q", ev.RRule())
	}
}

func TestParseEventExplicitDayList(t *testing.T) {
	now := time.Date(2026, 8, 26, 15, 0, 0, 0, time.UTC)
	ev, ok := parseEvent("gym 07:00-08:00 mon,wed,fri", now)
	if !ok {
		t.Fatal("not parsed")
	}
	if ev.Title != "gym" {
		t.Errorf("title = %q, want the day list stripped", ev.Title)
	}
	if len(ev.Days) != 3 {
		t.Errorf("days = %v", ev.Days)
	}
}

// A bare weekday names a recurrence day; "!fri" still means the coming Friday.
func TestDayNamesAndDeadlineTokensDoNotCollide(t *testing.T) {
	now := time.Date(2026, 8, 26, 15, 0, 0, 0, time.UTC) // Wednesday
	ev, ok := parseEvent("review 14:00-15:00 !fri", now)
	if !ok {
		t.Fatal("not parsed")
	}
	if len(ev.Days) != 0 {
		t.Errorf("days = %v, want none — !fri picks a date, not a day set", ev.Days)
	}
	if ev.Start.Weekday() != time.Friday || ev.Start.Day() != 28 {
		t.Errorf("start = %v, want the coming Friday", ev.Start)
	}
	if ev.Repeat != model.RepeatNone {
		t.Errorf("repeat = %q, want none", ev.Repeat)
	}
}
