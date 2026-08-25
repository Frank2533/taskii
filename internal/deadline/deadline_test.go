package deadline

import (
	"testing"
	"time"
)

// A Wednesday, mid-afternoon, so weekday and end-of-week maths are exercised
// away from any boundary.
func base(t *testing.T) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	return time.Date(2026, 8, 26, 15, 30, 0, 0, loc)
}

func TestParseStripsTokensFromTitle(t *testing.T) {
	now := base(t)
	title, spec := Parse("fix the zepto spider !today @3h", now)
	if title != "fix the zepto spider" {
		t.Errorf("title = %q, want the tokens removed", title)
	}
	if !spec.HasDue || !spec.HasRemind {
		t.Fatalf("spec = %+v, want both set", spec)
	}
}

// "Due today" is a promise about the day, not the current time of day.
func TestTodayMeansEndOfToday(t *testing.T) {
	now := base(t)
	_, spec := Parse("something !today", now)
	if !spec.HasDue {
		t.Fatal("no deadline parsed")
	}
	if h, m := spec.Due.Hour(), spec.Due.Minute(); h != 23 || m != 59 {
		t.Errorf("due = %v, want end of day", spec.Due)
	}
	if spec.Due.Day() != now.Day() {
		t.Errorf("due day = %d, want %d", spec.Due.Day(), now.Day())
	}
	if spec.Due.Location() != now.Location() {
		t.Errorf("due zone = %v, want the caller's zone", spec.Due.Location())
	}
}

func TestDayOffsets(t *testing.T) {
	now := base(t)
	cases := []struct {
		token string
		days  int
	}{
		{"!today", 0}, {"!0d", 0}, {"!tmr", 1}, {"!tomorrow", 1},
		{"!2d", 2}, {"!3day", 3}, {"!10days", 10},
	}
	for _, c := range cases {
		_, spec := Parse("x "+c.token, now)
		if !spec.HasDue {
			t.Errorf("%s: not parsed", c.token)
			continue
		}
		if got := daysBetween(now, spec.Due); got != c.days {
			t.Errorf("%s: %d days out, want %d", c.token, got, c.days)
		}
	}
}

// A reminder offset resolves against the moment it was entered, which is what
// the user means by "@3h".
func TestReminderIsRelativeToEntryTime(t *testing.T) {
	now := base(t)
	_, spec := Parse("x @3h", now)
	if !spec.HasRemind {
		t.Fatal("no reminder parsed")
	}
	if want := now.Add(3 * time.Hour); !spec.RemindAt.Equal(want) {
		t.Errorf("remind = %v, want %v", spec.RemindAt, want)
	}
	_, spec = Parse("x @90m", now)
	if want := now.Add(90 * time.Minute); !spec.RemindAt.Equal(want) {
		t.Errorf("remind = %v, want %v", spec.RemindAt, want)
	}
}

func TestWeekdayNamesResolveForward(t *testing.T) {
	now := base(t) // Wednesday
	_, spec := Parse("x !fri", now)
	if !spec.HasDue || spec.Due.Weekday() != time.Friday {
		t.Errorf("due = %v, want the coming Friday", spec.Due)
	}
	if got := daysBetween(now, spec.Due); got != 2 {
		t.Errorf("Friday is %d days out, want 2", got)
	}
	// Naming today's own weekday means today, not a week away.
	_, spec = Parse("x !wed", now)
	if got := daysBetween(now, spec.Due); got != 0 {
		t.Errorf("today's weekday resolved %d days out, want 0", got)
	}
}

func TestUnknownTokensStayInTheTitle(t *testing.T) {
	now := base(t)
	title, spec := Parse("email !bob about @home", now)
	if title != "email !bob about @home" {
		t.Errorf("title = %q, want it untouched", title)
	}
	if !spec.Empty() {
		t.Errorf("spec = %+v, want nothing parsed", spec)
	}
}

func TestDescribeMatchesTheAgreedWording(t *testing.T) {
	now := base(t)
	cases := []struct {
		days int
		want string
	}{
		{0, "should be completed today"},
		{1, "should be completed by tomorrow, end of day"},
	}
	for _, c := range cases {
		if got := Describe(InDays(now, c.days), now); got != c.want {
			t.Errorf("%d days: %q, want %q", c.days, got, c.want)
		}
	}
	if got := Describe(InDays(now, -1), now); got != "was due yesterday" {
		t.Errorf("yesterday: %q", got)
	}
}

func TestOverdueUsesTheDeadlineInstant(t *testing.T) {
	now := base(t)
	if Overdue(InDays(now, 0), now) {
		t.Error("a task due at end of today is not yet overdue at 15:30")
	}
	if !Overdue(InDays(now, -1), now) {
		t.Error("yesterday's deadline should be overdue")
	}
}

func TestEndOfWeekLandsOnSunday(t *testing.T) {
	now := base(t)
	got := EndOfWeek(now)
	if got.Weekday() != time.Sunday {
		t.Errorf("end of week = %v, want a Sunday", got.Weekday())
	}
}
