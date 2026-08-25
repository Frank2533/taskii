package notify

import (
	"strings"
	"testing"
	"time"
)

func TestLogRoundTripsAndKeepsOrder(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 8, 25, 13, 45, 0, 0, time.UTC)
	for i, o := range []Outcome{Sent, Failed, Skipped} {
		err := Append(dir, Entry{
			At:    base.Add(time.Duration(i) * time.Minute),
			Title: "taskii", Message: "standup in 5 minutes", Outcome: o,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := ReadLog(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("entries = %d, want 3", len(got))
	}
	if got[0].Outcome != Sent || got[2].Outcome != Skipped {
		t.Errorf("order lost: %v", []Outcome{got[0].Outcome, got[2].Outcome})
	}
}

func TestLogIsCapped(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	for i := 0; i < maxLogEntries+20; i++ {
		if err := Append(dir, Entry{At: base.Add(time.Duration(i) * time.Second), Message: "x", Outcome: Sent}); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := ReadLog(dir, 0)
	if len(got) != maxLogEntries {
		t.Errorf("entries = %d, want the cap of %d", len(got), maxLogEntries)
	}
	// The oldest are the ones dropped.
	if !got[0].At.After(base) {
		t.Error("the newest entries were dropped instead of the oldest")
	}
}

func TestReadLogLimit(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		Append(dir, Entry{At: base.Add(time.Duration(i) * time.Minute), Message: "x", Outcome: Sent})
	}
	got, _ := ReadLog(dir, 3)
	if len(got) != 3 {
		t.Fatalf("entries = %d, want 3", len(got))
	}
	if !got[2].At.Equal(base.Add(9 * time.Minute)) {
		t.Error("the limit returned the oldest rather than the most recent")
	}
}

func TestMissingLogIsNotAnError(t *testing.T) {
	got, err := ReadLog(t.TempDir(), 0)
	if err != nil || got != nil {
		t.Errorf("got %v, %v; want nothing and no error", got, err)
	}
}

// A skipped delivery must say why, or a missing notification is
// indistinguishable from a broken one.
func TestSkippedEntryCarriesItsReason(t *testing.T) {
	dir := t.TempDir()
	Append(dir, Entry{
		At: time.Now(), Message: "standup in 15 minutes",
		Outcome: Skipped, Detail: "taskii was not running when it was due",
	})
	got, _ := ReadLog(dir, 1)
	if len(got) != 1 {
		t.Fatal("nothing logged")
	}
	if !strings.Contains(got[0].Line(), "not running") {
		t.Errorf("the reason is not shown: %q", got[0].Line())
	}
}
