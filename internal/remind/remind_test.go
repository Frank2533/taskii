package remind

import (
	"testing"
	"time"

	"taskii/internal/model"
)

func standup(start time.Time) model.Event {
	return model.Event{
		ID: "e1", Title: "standup",
		Start: start, End: start.Add(15 * time.Minute),
		Repeat: model.RepeatWeekly, Days: model.Weekdays,
	}
}

func TestEachLeadFiresOnceAtItsTime(t *testing.T) {
	start := time.Date(2026, 8, 26, 9, 30, 0, 0, time.UTC) // Wednesday
	e := standup(start)
	fired := NewFired()

	for _, lead := range Leads {
		now := start.Add(-lead)
		got := Pending([]model.Event{e}, now, fired)
		if len(got) != 1 {
			t.Fatalf("at -%v got %d reminders, want 1", lead, len(got))
		}
		if got[0].Lead != lead {
			t.Errorf("lead = %v, want %v", got[0].Lead, lead)
		}
		// A second sweep a moment later must not repeat it.
		if again := Pending([]model.Event{e}, now.Add(30*time.Second), fired); len(again) != 0 {
			t.Errorf("reminder repeated: %+v", again)
		}
	}
}

func TestMessageWording(t *testing.T) {
	start := time.Date(2026, 8, 26, 9, 30, 0, 0, time.UTC)
	d := Due{Title: "standup", Lead: time.Minute, EventStart: start}
	if got := d.Message(); got != "standup in 1 minute (09:30)" {
		t.Errorf("message = %q", got)
	}
	d.Lead = 15 * time.Minute
	if got := d.Message(); got != "standup in 15 minutes (09:30)" {
		t.Errorf("message = %q", got)
	}
}

// A "15 minutes before" warning arriving three minutes before is wrong about
// the thing it exists to say, so a missed reminder is dropped, not delivered
// late — and not delivered at the next sweep either.
func TestLateRemindersAreSkippedNotQueued(t *testing.T) {
	start := time.Date(2026, 8, 26, 9, 30, 0, 0, time.UTC)
	e := standup(start)
	fired := NewFired()

	// The app was closed and opens three minutes before the event: the 15 and
	// 5 minute reminders are long past.
	got := Pending([]model.Event{e}, start.Add(-3*time.Minute), fired)
	if len(got) != 0 {
		t.Fatalf("delivered stale reminders: %+v", got)
	}
	// The 1-minute reminder is still ahead and must still fire.
	got = Pending([]model.Event{e}, start.Add(-1*time.Minute), fired)
	if len(got) != 1 || got[0].Lead != time.Minute {
		t.Errorf("the 1-minute reminder did not fire: %+v", got)
	}
}

// A repeating event warns again on its next occurrence.
func TestNextOccurrenceRemindsAgain(t *testing.T) {
	start := time.Date(2026, 8, 26, 9, 30, 0, 0, time.UTC) // Wednesday
	e := standup(start)
	fired := NewFired()

	if got := Pending([]model.Event{e}, start.Add(-time.Minute), fired); len(got) != 1 {
		t.Fatalf("first occurrence did not remind: %+v", got)
	}
	tomorrow := start.AddDate(0, 0, 1) // Thursday, a weekday
	if got := Pending([]model.Event{e}, tomorrow.Add(-time.Minute), fired); len(got) != 1 {
		t.Errorf("next occurrence did not remind: %+v", got)
	}
}

func TestPruneDropsOldMarkers(t *testing.T) {
	start := time.Date(2026, 8, 26, 9, 30, 0, 0, time.UTC)
	fired := NewFired()
	Pending([]model.Event{standup(start)}, start.Add(-time.Minute), fired)
	if len(fired.Keys) == 0 {
		t.Fatal("nothing was recorded")
	}
	fired.Prune(start.Add(48 * time.Hour))
	if len(fired.Keys) != 0 {
		t.Errorf("old markers survived pruning: %v", fired.Keys)
	}
}

func TestFiredStoreRoundTrips(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 8, 26, 9, 30, 0, 0, time.UTC)
	fired := NewFired()
	Pending([]model.Event{standup(start)}, start.Add(-time.Minute), fired)
	if err := fired.Save(dir); err != nil {
		t.Fatal(err)
	}

	// Restarting must not replay what already fired.
	reloaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := Pending([]model.Event{standup(start)}, start.Add(-50*time.Second), reloaded); len(got) != 0 {
		t.Errorf("restart replayed a fired reminder: %+v", got)
	}
}
