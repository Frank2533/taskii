package ui

import (
	"strings"
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

// Editing shows exactly what would have created the event, so there is one
// grammar to learn and no second representation to keep in step.
func TestFormatEventRoundTripsThroughTheParser(t *testing.T) {
	now := time.Date(2026, 8, 26, 15, 0, 0, 0, time.UTC)
	for _, input := range []string{
		"standup 09:30-09:45 weekdays",
		"gym 07:00-08:00 mon,wed,fri",
		"planning 10:00-11:00 monthly x2",
		"review 14:00-15:30 !fri",
		"lunch 12:00-13:00",
	} {
		first, ok := parseEvent(input, now)
		if !ok {
			t.Fatalf("%q did not parse", input)
		}
		rendered := formatEvent(first, now)
		second, ok := parseEvent(rendered, now)
		if !ok {
			t.Fatalf("%q rendered to %q, which does not parse", input, rendered)
		}
		if second.Title != first.Title {
			t.Errorf("%q: title %q became %q", input, first.Title, second.Title)
		}
		if !second.Start.Equal(first.Start) || !second.End.Equal(first.End) {
			t.Errorf("%q -> %q: times %v/%v became %v/%v",
				input, rendered, first.Start, first.End, second.Start, second.End)
		}
		if second.Repeat != first.Repeat {
			t.Errorf("%q -> %q: repeat %q became %q", input, rendered, first.Repeat, second.Repeat)
		}
		if len(second.Days) != len(first.Days) {
			t.Errorf("%q -> %q: days %v became %v", input, rendered, first.Days, second.Days)
		}
		if second.Interval != first.Interval {
			t.Errorf("%q -> %q: interval %d became %d", input, rendered, first.Interval, second.Interval)
		}
	}
}

func calendarWithEvent(t *testing.T, text string) App {
	t.Helper()
	a, _ := dashApp(t)
	a = press(t, a, "3")
	a = press(t, a, "a")
	a.input.SetValue(text)
	a = press(t, a, "enter")
	if a.err != "" {
		t.Fatalf("add reported: %s", a.err)
	}
	// Put the cursor on the day the event lands on, then onto the event
	// itself: a day can also hold tickets and tasks, and entry zero is
	// whatever sorts first.
	a.calCursor = a.events[0].Start
	a.calEntrySel = entryIndexOf(t, a, a.events[0].ID)
	return a
}

// entryIndexOf finds an event's position among the selected day's entries.
func entryIndexOf(t *testing.T, a App, id string) int {
	t.Helper()
	for i, e := range a.entriesOnSelectedDay() {
		if e.eventID == id {
			return i
		}
	}
	t.Fatalf("event %q is not on the selected day", id)
	return 0
}

func TestEditEventUpdatesInPlace(t *testing.T) {
	a := calendarWithEvent(t, "standup 09:30-09:45 weekdays")
	original := a.events[0].ID

	a = press(t, a, "e")
	if a.mode != modeEditEvent {
		t.Fatalf("mode = %v, want modeEditEvent", a.mode)
	}
	if !strings.Contains(a.input.Value(), "standup") {
		t.Errorf("input seeded with %q, want the event", a.input.Value())
	}
	if !strings.Contains(a.View(), "standup") {
		t.Error("the edit input is not rendered")
	}

	a.input.SetValue("standup 10:00-10:15 weekdays")
	a = press(t, a, "enter")
	// It repeats, so the scope has to be answered before anything changes.
	if !a.scope.open {
		t.Fatal("no scope prompt for a repeating event")
	}
	a = press(t, a, "a") // the whole series
	if a.err != "" {
		t.Fatalf("edit reported: %s", a.err)
	}
	if len(a.events) != 1 {
		t.Fatalf("events = %d, want the one edited in place", len(a.events))
	}
	if a.events[0].Start.Format("15:04") != "10:00" {
		t.Errorf("start = %s, want 10:00", a.events[0].Start.Format("15:04"))
	}
	// Keeping the id keeps it the same entry for subscribers and for reminders
	// already sent.
	if a.events[0].ID != original {
		t.Errorf("id changed from %q to %q", original, a.events[0].ID)
	}
}

func TestEditRefusesToDropTheTimeRange(t *testing.T) {
	a := calendarWithEvent(t, "standup 09:30-09:45 weekdays")
	a = press(t, a, "e")
	a.input.SetValue("standup weekdays")
	a = press(t, a, "enter")

	if a.scope.open {
		t.Fatal("a rejected input reached the scope prompt")
	}
	if a.err == "" {
		t.Error("expected a complaint about the missing time range")
	}
	if a.events[0].Start.Format("15:04") != "09:30" {
		t.Error("the event was changed despite the input being rejected")
	}
}

// Only events are editable here; tasks and tickets are edited where they live.
func TestEditSaysSoWhenTheCursorIsNotOnAnEvent(t *testing.T) {
	a, _ := dashApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "buy oat milk !today")
	a = press(t, a, "enter")
	a = press(t, a, "3")
	a.calCursor = a.now()

	a = press(t, a, "e")
	if a.mode == modeEditEvent {
		t.Fatal("started editing something that is not an event")
	}
	if a.err == "" {
		t.Error("no explanation was given")
	}
}

func TestDeleteRemovesTheSelectedEvent(t *testing.T) {
	a := calendarWithEvent(t, "standup 09:30-09:45 weekdays")
	a = press(t, a, "d")
	if !a.scope.open {
		t.Fatal("no scope prompt for a repeating event")
	}
	a = press(t, a, "a") // the whole series
	if a.err != "" {
		t.Fatalf("delete reported: %s", a.err)
	}
	if len(a.events) != 0 {
		t.Errorf("events = %d, want none", len(a.events))
	}
}

func TestTabStepsThroughADaysEntries(t *testing.T) {
	a := calendarWithEvent(t, "standup 09:30-09:45")
	day := a.events[0].Start
	a = press(t, a, "a")
	a.input.SetValue("retro 16:00-17:00 !" + day.Format("2006-01-02"))
	a = press(t, a, "enter")
	if a.err != "" {
		t.Fatalf("second add reported: %s", a.err)
	}

	entries := a.entriesOnSelectedDay()
	if len(entries) < 2 {
		t.Fatalf("entries on the day = %d, want at least the two events", len(entries))
	}
	// Stepping the whole way round must visit every entry and come back.
	seen := map[string]bool{}
	for range entries {
		e, ok := a.selectedEntry()
		if !ok {
			t.Fatal("nothing selected")
		}
		seen[e.title] = true
		a = press(t, a, "tab")
	}
	if len(seen) != len(entries) {
		t.Errorf("tab visited %d of %d entries: %v", len(seen), len(entries), seen)
	}
	if _, ok := a.selectedEvent(); !ok {
		// After a full cycle the selection is back where it started, which
		// was the event.
		t.Error("a full cycle did not return to the starting entry")
	}
}

// A one-off event has no ambiguity, so it must not ask.
func TestNonRepeatingEventEditsWithoutAsking(t *testing.T) {
	a := calendarWithEvent(t, "lunch 12:00-13:00")
	a = press(t, a, "e")
	a.input.SetValue("lunch 12:30-13:30")
	a = press(t, a, "enter")

	if a.scope.open {
		t.Fatal("asked about scope for a one-off event")
	}
	if a.events[0].Start.Format("15:04") != "12:30" {
		t.Errorf("start = %s, want 12:30", a.events[0].Start.Format("15:04"))
	}
}

// Editing one occurrence must leave the series itself alone.
func TestEditThisOccurrenceOnlySplitsItOut(t *testing.T) {
	a := calendarWithEvent(t, "standup 09:30-09:45 weekdays")
	series := a.events[0]
	occ := a.scopeOccurrence(series)

	a = press(t, a, "e")
	a.input.SetValue("standup 11:00-11:30 weekdays")
	a = press(t, a, "enter")
	a = press(t, a, "t") // this occurrence only

	if len(a.events) != 2 {
		t.Fatalf("events = %d, want the series plus a one-off", len(a.events))
	}
	var kept, one model.Event
	for _, e := range a.events {
		if e.ID == series.ID {
			kept = e
		} else {
			one = e
		}
	}
	if kept.Start.Format("15:04") != "09:30" {
		t.Errorf("the series moved to %s; it should be untouched", kept.Start.Format("15:04"))
	}
	if len(kept.Except) != 1 {
		t.Errorf("the occurrence was not excluded: %v", kept.Except)
	}
	if one.Repeat != model.RepeatNone {
		t.Errorf("the split-out occurrence still repeats (%q)", one.Repeat)
	}
	if one.Start.Format("15:04") != "11:00" {
		t.Errorf("the split-out occurrence starts %s, want 11:00", one.Start.Format("15:04"))
	}

	// The excluded day must now show the one-off and not the series.
	day := startOfDay(occ)
	occs := model.EventsOccurring(a.events, day, day.AddDate(0, 0, 1))
	if len(occs) != 1 {
		t.Fatalf("that day has %d occurrences, want exactly the replacement", len(occs))
	}
	if occs[0].Start.Format("15:04") != "11:00" {
		t.Errorf("that day shows %s, want the replacement", occs[0].Start.Format("15:04"))
	}
}

// "This and future" splits the series, leaving earlier occurrences as they were.
func TestEditFutureSplitsTheSeries(t *testing.T) {
	a := calendarWithEvent(t, "standup 09:30-09:45 weekdays")
	series := a.events[0]

	// Move the cursor a week on, so there is a past to preserve.
	later := series.Start.AddDate(0, 0, 7)
	a.calCursor = later
	a.calEntrySel = entryIndexOf(t, a, series.ID)

	a = press(t, a, "e")
	a.input.SetValue("standup 10:00-10:15 weekdays")
	a = press(t, a, "enter")
	a = press(t, a, "f") // this and future

	if len(a.events) != 2 {
		t.Fatalf("events = %d, want the bounded original plus its successor", len(a.events))
	}
	var old, rest model.Event
	for _, e := range a.events {
		if e.ID == series.ID {
			old = e
		} else {
			rest = e
		}
	}
	if old.Until == nil {
		t.Fatal("the original series was not bounded")
	}
	if !old.Until.Before(later) {
		t.Errorf("the original runs to %v, which is not before the split at %v", old.Until, later)
	}
	if rest.Start.Format("15:04") != "10:00" {
		t.Errorf("the new series starts %s, want 10:00", rest.Start.Format("15:04"))
	}

	// The first occurrence, a week earlier, must still be at the old time.
	day := startOfDay(series.Start)
	occs := model.EventsOccurring(a.events, day, day.AddDate(0, 0, 1))
	if len(occs) != 1 || occs[0].Start.Format("15:04") != "09:30" {
		t.Errorf("the past was rewritten: %+v", occs)
	}
}

// Cancelling one occurrence of a repeat removes only that day.
func TestDeleteThisOccurrenceOnly(t *testing.T) {
	a := calendarWithEvent(t, "standup 09:30-09:45 weekdays")
	series := a.events[0]
	occ := a.scopeOccurrence(series)

	a = press(t, a, "d")
	a = press(t, a, "t")

	if len(a.events) != 1 {
		t.Fatalf("events = %d, want the series kept", len(a.events))
	}
	day := startOfDay(occ)
	if occs := model.EventsOccurring(a.events, day, day.AddDate(0, 0, 1)); len(occs) != 0 {
		t.Errorf("that day still has %d occurrences", len(occs))
	}
	// The next weekday must be unaffected.
	next := day.AddDate(0, 0, 1)
	if occs := model.EventsOccurring(a.events, next, next.AddDate(0, 0, 1)); len(occs) == 0 {
		t.Error("the rest of the series was removed too")
	}
}

func TestScopePromptCanBeCancelled(t *testing.T) {
	a := calendarWithEvent(t, "standup 09:30-09:45 weekdays")
	a = press(t, a, "d")
	a = press(t, a, "esc")

	if a.scope.open {
		t.Error("the prompt stayed open")
	}
	if len(a.events) != 1 {
		t.Errorf("events = %d, want the event untouched", len(a.events))
	}
}
