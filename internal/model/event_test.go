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
