package vault

import (
	"strings"
	"testing"
	"time"
)

func TestParseTaskLineStripsTokens(t *testing.T) {
	text, meta := ParseTaskLine("check zepto spider 📅 2026-08-27 (@2026-08-26 09:00)", time.UTC)
	if text != "check zepto spider" {
		t.Errorf("text = %q, want the tokens removed", text)
	}
	if !meta.HasDue || meta.Due.Format("2006-01-02") != "2026-08-27" {
		t.Errorf("due = %v", meta.Due)
	}
	// A due date is a whole day, so it falls due at the end of it.
	if meta.Due.Hour() != 23 {
		t.Errorf("due hour = %d, want end of day", meta.Due.Hour())
	}
	if !meta.HasRemind || meta.RemindAt.Format("15:04") != "09:00" {
		t.Errorf("remind = %v", meta.RemindAt)
	}
}

func TestFormatTaskLineRoundTrips(t *testing.T) {
	orig := TaskMeta{
		Due: time.Date(2026, 8, 27, 23, 59, 59, 0, time.UTC), HasDue: true,
		RemindAt: time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC), HasRemind: true,
	}
	line := FormatTaskLine("write it up", orig)
	text, back := ParseTaskLine(line, time.UTC)
	if text != "write it up" {
		t.Errorf("text = %q", text)
	}
	if !back.HasDue || !back.Due.Equal(orig.Due) {
		t.Errorf("due did not round trip: %v vs %v", back.Due, orig.Due)
	}
	if !back.HasRemind || !back.RemindAt.Equal(orig.RemindAt) {
		t.Errorf("reminder did not round trip: %v vs %v", back.RemindAt, orig.RemindAt)
	}
}

func TestSetTaskScheduleWritesIntoTheLine(t *testing.T) {
	path := tempNote(t, ticketNote)
	due := time.Date(2026, 8, 27, 23, 59, 59, 0, time.UTC)
	err := SetTaskSchedule(path, 9, "open one", TaskMeta{Due: due, HasDue: true}, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	body := read(t, path)
	if !strings.Contains(body, "- [ ] open one 📅 2026-08-27") {
		t.Errorf("schedule not written:\n%s", body)
	}
	// The checked state and the other task must be untouched.
	if !strings.Contains(body, "- [x] done one") {
		t.Error("the sibling task was disturbed")
	}
}

// Setting a schedule twice must replace the tokens, not stack them up.
func TestSetTaskScheduleReplacesExistingTokens(t *testing.T) {
	path := tempNote(t, ticketNote)
	first := time.Date(2026, 8, 27, 23, 59, 59, 0, time.UTC)
	second := time.Date(2026, 9, 1, 23, 59, 59, 0, time.UTC)
	if err := SetTaskSchedule(path, 9, "open one", TaskMeta{Due: first, HasDue: true}, time.UTC); err != nil {
		t.Fatal(err)
	}
	// The stored text now carries a token; the caller passes the bare text.
	if err := SetTaskSchedule(path, 9, "open one", TaskMeta{Due: second, HasDue: true}, time.UTC); err != nil {
		t.Fatal(err)
	}
	body := read(t, path)
	if strings.Count(body, DueEmoji) != 1 {
		t.Errorf("tokens accumulated:\n%s", body)
	}
	if !strings.Contains(body, "2026-09-01") {
		t.Errorf("date not updated:\n%s", body)
	}
}

func TestClearingScheduleRemovesTokens(t *testing.T) {
	path := tempNote(t, ticketNote)
	due := time.Date(2026, 8, 27, 23, 59, 59, 0, time.UTC)
	if err := SetTaskSchedule(path, 9, "open one", TaskMeta{Due: due, HasDue: true}, time.UTC); err != nil {
		t.Fatal(err)
	}
	if err := SetTaskSchedule(path, 9, "open one", TaskMeta{}, time.UTC); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(t, path), DueEmoji) {
		t.Errorf("tokens survived being cleared:\n%s", read(t, path))
	}
}
