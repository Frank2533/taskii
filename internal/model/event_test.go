package model

import (
	"testing"
	"time"
)

func at(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, time.UTC)
}

func standup() Event {
	return Event{
		ID:     "e1",
		Title:  "standup",
		Start:  at(2026, time.August, 24, 9, 30), // a Monday
		End:    at(2026, time.August, 24, 9, 45),
		Repeat: RepeatWeekly,
	}
}

func TestSingleEventOnlyAppearsInItsRange(t *testing.T) {
	e := Event{Start: at(2026, time.August, 25, 10, 0), End: at(2026, time.August, 25, 11, 0)}
	if got := e.Occurrences(at(2026, time.August, 25, 0, 0), at(2026, time.August, 26, 0, 0)); len(got) != 1 {
		t.Errorf("same day = %d occurrences, want 1", len(got))
	}
	if got := e.Occurrences(at(2026, time.August, 26, 0, 0), at(2026, time.August, 27, 0, 0)); len(got) != 0 {
		t.Errorf("next day = %d occurrences, want 0", len(got))
	}
}

// An event that started before the window but runs into it must still show.
func TestOverlappingEventIsIncluded(t *testing.T) {
	e := Event{Start: at(2026, time.August, 25, 23, 0), End: at(2026, time.August, 26, 1, 0)}
	got := e.Occurrences(at(2026, time.August, 26, 0, 0), at(2026, time.August, 27, 0, 0))
	if len(got) != 1 {
		t.Errorf("overlap = %d occurrences, want 1", len(got))
	}
}

func TestWeeklyRepeatLandsOnTheSameWeekday(t *testing.T) {
	e := standup()
	got := e.Occurrences(at(2026, time.August, 24, 0, 0), at(2026, time.September, 21, 0, 0))
	if len(got) != 4 {
		t.Fatalf("occurrences = %d, want 4 weeks", len(got))
	}
	for _, o := range got {
		if o.Start.Weekday() != time.Monday {
			t.Errorf("occurrence on %v, want Monday", o.Start.Weekday())
		}
		if o.End.Sub(o.Start) != 15*time.Minute {
			t.Errorf("duration = %v, want the original 15m", o.End.Sub(o.Start))
		}
	}
}

func TestIntervalSkipsPeriods(t *testing.T) {
	e := standup()
	e.Interval = 2
	got := e.Occurrences(at(2026, time.August, 24, 0, 0), at(2026, time.September, 21, 0, 0))
	if len(got) != 2 {
		t.Errorf("every other week = %d occurrences, want 2", len(got))
	}
}

func TestUntilBoundsARepeat(t *testing.T) {
	e := standup()
	until := at(2026, time.September, 1, 0, 0)
	e.Until = &until
	got := e.Occurrences(at(2026, time.August, 24, 0, 0), at(2026, time.October, 1, 0, 0))
	if len(got) != 2 {
		t.Errorf("bounded repeat = %d occurrences, want 2 (24 and 31 Aug)", len(got))
	}
}

// An unbounded daily event must not spin forever when asked for a wide range.
func TestUnboundedRepeatIsCapped(t *testing.T) {
	e := Event{Start: at(2020, time.January, 1, 9, 0), End: at(2020, time.January, 1, 10, 0), Repeat: RepeatDaily}
	got := e.Occurrences(at(2020, time.January, 1, 0, 0), at(2100, time.January, 1, 0, 0))
	if len(got) != maxOccurrences {
		t.Errorf("occurrences = %d, want the cap of %d", len(got), maxOccurrences)
	}
}

func TestOccurrencesAreSortedAcrossEvents(t *testing.T) {
	late := Event{ID: "b", Start: at(2026, time.August, 25, 15, 0), End: at(2026, time.August, 25, 16, 0)}
	early := Event{ID: "a", Start: at(2026, time.August, 25, 9, 0), End: at(2026, time.August, 25, 10, 0)}
	got := EventsOccurring([]Event{late, early}, at(2026, time.August, 25, 0, 0), at(2026, time.August, 26, 0, 0))
	if len(got) != 2 {
		t.Fatalf("occurrences = %d", len(got))
	}
	if got[0].Event.ID != "a" {
		t.Error("occurrences are not in start order")
	}
}

func TestRRuleRendersTheRepeat(t *testing.T) {
	e := standup()
	if got := e.RRule(); got != "FREQ=WEEKLY" {
		t.Errorf("RRule = %q", got)
	}
	e.Interval = 2
	until := at(2026, time.September, 1, 0, 0)
	e.Until = &until
	if got := e.RRule(); got != "FREQ=WEEKLY;INTERVAL=2;UNTIL=20260901T000000Z" {
		t.Errorf("RRule = %q", got)
	}
	if got := (Event{}).RRule(); got != "" {
		t.Errorf("a non-repeating event produced %q", got)
	}
}

func TestDurationDefaultsWhenEndIsMissing(t *testing.T) {
	e := Event{Start: at(2026, time.August, 25, 9, 0)}
	if e.Duration() != time.Hour {
		t.Errorf("duration = %v, want an hour", e.Duration())
	}
}

// A standup that runs Monday to Friday is the reason day sets exist.
func TestWeekdaysRepeatSkipsTheWeekend(t *testing.T) {
	e := Event{
		Title:  "standup",
		Start:  at(2026, time.August, 26, 9, 30), // a Wednesday
		End:    at(2026, time.August, 26, 9, 45),
		Repeat: RepeatWeekly,
		Days:   Weekdays,
	}
	// One full week from the Monday of that week.
	got := e.Occurrences(at(2026, time.August, 24, 0, 0), at(2026, time.August, 31, 0, 0))
	if len(got) != 3 {
		// Starts Wednesday, so Mon and Tue of that week are before the start.
		t.Fatalf("occurrences = %d, want Wed/Thu/Fri: %v", len(got), starts(got))
	}
	for _, o := range got {
		if o.Start.Weekday() == time.Saturday || o.Start.Weekday() == time.Sunday {
			t.Errorf("occurrence on a %v", o.Start.Weekday())
		}
		if o.Start.Format("15:04") != "09:30" {
			t.Errorf("time of day = %s, want 09:30", o.Start.Format("15:04"))
		}
	}
}

// The event's own weekday must not restrict the set: a standup created on a
// Wednesday still runs on the Monday after.
func TestWeekdaysRepeatCoversTheFollowingWeekInFull(t *testing.T) {
	e := Event{
		Start:  at(2026, time.August, 26, 9, 30), // Wednesday
		End:    at(2026, time.August, 26, 9, 45),
		Repeat: RepeatWeekly,
		Days:   Weekdays,
	}
	got := e.Occurrences(at(2026, time.August, 31, 0, 0), at(2026, time.September, 7, 0, 0))
	if len(got) != 5 {
		t.Fatalf("next week = %d occurrences, want 5: %v", len(got), starts(got))
	}
	if got[0].Start.Weekday() != time.Monday {
		t.Errorf("first = %v, want Monday", got[0].Start.Weekday())
	}
}

func TestArbitraryDaySet(t *testing.T) {
	e := Event{
		Start:  at(2026, time.August, 24, 10, 0), // Monday
		End:    at(2026, time.August, 24, 11, 0),
		Repeat: RepeatWeekly,
		Days:   []time.Weekday{time.Monday, time.Wednesday, time.Friday},
	}
	got := e.Occurrences(at(2026, time.August, 24, 0, 0), at(2026, time.August, 31, 0, 0))
	if len(got) != 3 {
		t.Fatalf("occurrences = %d, want 3: %v", len(got), starts(got))
	}
	want := []time.Weekday{time.Monday, time.Wednesday, time.Friday}
	for i, o := range got {
		if o.Start.Weekday() != want[i] {
			t.Errorf("occurrence %d on %v, want %v", i, o.Start.Weekday(), want[i])
		}
	}
}

// Every other week still means every other week when days are named.
func TestDaySetRespectsInterval(t *testing.T) {
	e := Event{
		Start:    at(2026, time.August, 24, 10, 0),
		End:      at(2026, time.August, 24, 11, 0),
		Repeat:   RepeatWeekly,
		Interval: 2,
		Days:     []time.Weekday{time.Monday, time.Wednesday},
	}
	// Active weeks are 24 Aug and 7 Sep; 31 Aug is skipped. The window has to
	// reach past 7 Sep for the second active week to fall inside it.
	got := e.Occurrences(at(2026, time.August, 24, 0, 0), at(2026, time.September, 14, 0, 0))
	if len(got) != 4 {
		t.Fatalf("occurrences = %d, want 4 across two active weeks: %v", len(got), starts(got))
	}
	// The skipped week must contribute nothing.
	for _, o := range got {
		if d := o.Start.Day(); d == 31 || d == 2 {
			t.Errorf("occurrence in the skipped week: %v", o.Start)
		}
	}
}

func TestDaySetRendersByDayRule(t *testing.T) {
	e := Event{Start: at(2026, time.August, 26, 9, 30), Repeat: RepeatWeekly, Days: Weekdays}
	if got := e.RRule(); got != "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR" {
		t.Errorf("RRule = %q", got)
	}
	e.Days = []time.Weekday{time.Friday, time.Monday}
	if got := e.RRule(); got != "FREQ=WEEKLY;BYDAY=MO,FR" {
		t.Errorf("RRule = %q, want days ordered from Monday", got)
	}
}

func starts(occ []Occurrence) []string {
	var out []string
	for _, o := range occ {
		out = append(out, o.Start.Format("Mon 02 Jan 15:04"))
	}
	return out
}
