package ics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sample(loc *time.Location) *Calendar {
	stamp := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	return &Calendar{
		Name: "NIQ Work",
		Loc:  loc,
		Events: []Event{
			{
				UID: UID("ticket", "SCRAP-2"), Summary: "second", AllDay: true,
				Start: time.Date(2026, 8, 26, 0, 0, 0, 0, loc), Stamp: stamp,
			},
			{
				UID: UID("ticket", "SCRAP-1"), Summary: "first", AllDay: true,
				Start: time.Date(2026, 8, 25, 0, 0, 0, 0, loc), Stamp: stamp,
			},
			{
				UID: UID("appt", "a1"), Summary: "standup",
				Start: time.Date(2026, 8, 25, 9, 30, 0, 0, loc), Stamp: stamp,
			},
		},
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	loc := time.UTC
	first := sample(loc).Render()
	// Feed the events in a different order; output must not move.
	c := sample(loc)
	c.Events[0], c.Events[2] = c.Events[2], c.Events[0]
	second := c.Render()
	if string(first) != string(second) {
		t.Error("render depends on input order — this would churn the vault on every export")
	}
	third := sample(loc).Render()
	if string(first) != string(third) {
		t.Error("render is not stable across calls")
	}
}

func TestRenderUsesCRLFAndRequiredProperties(t *testing.T) {
	out := string(sample(time.UTC).Render())
	if !strings.HasPrefix(out, "BEGIN:VCALENDAR\r\n") {
		t.Error("lines must terminate with CRLF")
	}
	for _, want := range []string{"VERSION:2.0", "PRODID:" + ProdID, "END:VCALENDAR"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Count(out, "BEGIN:VEVENT") != 3 {
		t.Errorf("expected 3 events, got %d", strings.Count(out, "BEGIN:VEVENT"))
	}
}

// A due date is a calendar day, not an instant. Encoding it as a timed event
// would shift it across midnight for anyone far enough from UTC.
func TestAllDayCarriesNoTime(t *testing.T) {
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	out := string(sample(kolkata).Render())
	if !strings.Contains(out, "DTSTART;VALUE=DATE:20260825") {
		t.Errorf("all-day start missing or shifted:\n%s", out)
	}
	// DTEND is exclusive for dates.
	if !strings.Contains(out, "DTEND;VALUE=DATE:20260826") {
		t.Errorf("all-day end should be the next day:\n%s", out)
	}
}

func TestTimedEventConvertsToUTC(t *testing.T) {
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	out := string(sample(kolkata).Render())
	// 09:30 IST is 04:00Z.
	if !strings.Contains(out, "DTSTART:20260825T040000Z") {
		t.Errorf("timed event not converted from the configured zone:\n%s", out)
	}
	if !strings.Contains(out, "DTEND:20260825T043000Z") {
		t.Errorf("default duration not applied:\n%s", out)
	}
}

func TestStampNeverUsesTheClock(t *testing.T) {
	c := sample(time.UTC)
	for i := range c.Events {
		c.Events[i].Stamp = time.Time{}
	}
	first := string(c.Render())
	second := string(c.Render())
	if first != second {
		t.Error("zero DTSTAMP must render a fixed value, not the current time")
	}
	if strings.Contains(first, time.Now().UTC().Format("20060102T15")) {
		t.Error("DTSTAMP fell back to the current clock")
	}
}

func TestEscapingAndFolding(t *testing.T) {
	long := strings.Repeat("word ", 40)
	c := &Calendar{Loc: time.UTC, Events: []Event{{
		UID: "x@taskii", Summary: "a; b, c\\ d\nnewline " + long, AllDay: true,
		Start: time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
	}}}
	out := string(c.Render())
	if !strings.Contains(out, "a\\; b\\, c\\\\ d\\nnewline") {
		t.Errorf("special characters not escaped:\n%s", out)
	}
	for _, line := range strings.Split(out, "\r\n") {
		if len(line) > 75 {
			t.Errorf("line exceeds 75 octets (%d): %q", len(line), line)
		}
	}
	if !strings.Contains(out, "\r\n ") {
		t.Error("long line was not folded")
	}
}

func TestWriteIfChangedSkipsIdenticalContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "cal.ics")
	data := sample(time.UTC).Render()

	wrote, err := WriteIfChanged(path, data)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("first write should have happened")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	before := info.ModTime()

	wrote, err = WriteIfChanged(path, data)
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Error("identical content was rewritten — this is what churns the vault")
	}
	info, _ = os.Stat(path)
	if !info.ModTime().Equal(before) {
		t.Error("file was touched despite identical content")
	}

	if wrote, err = WriteIfChanged(path, append(data, []byte("X")...)); err != nil || !wrote {
		t.Errorf("changed content should write: wrote=%v err=%v", wrote, err)
	}
}

func TestUIDIsStable(t *testing.T) {
	if UID("ticket", "SCRAP-1") != UID("ticket", "scrap-1") {
		t.Error("UID must not vary with case, or re-export duplicates the event")
	}
}
