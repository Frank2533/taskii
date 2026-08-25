package ui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"taskii/internal/model"
	"taskii/internal/para"
)

// subtaskReminderApp puts a reminder on a subtask in the fixture vault and
// indexes it, as setting one through the picker would.
func subtaskReminderApp(t *testing.T, srvURL string, remindAt, now time.Time) (App, string) {
	t.Helper()
	a, root := dashApp(t)
	a.noPersist = false
	a.now = func() time.Time { return now }
	a.pushEnabled = srvURL != ""
	a.ntfyServer = srvURL
	a.ntfyTopic = "test-topic"

	notePath := filepath.Join(root, "Tickets", "AAA-1 In Progress live.md")
	body, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	stamped := strings.Replace(string(body), "- [ ] check zepto spider",
		"- [ ] check zepto spider (@"+remindAt.Format("2006-01-02 15:04")+")", 1)
	if err := os.WriteFile(notePath, []byte(stamped), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := para.Scan(root, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := a.Update(indexMsg{idx: idx})
	return m.(App), root
}

// A reminder written into a task line must actually fire. The sweep only ever
// walked taskii's own task list, so these were parsed, displayed, and never
// delivered.
func TestSubtaskReminderFires(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(c.handler))
	defer srv.Close()

	model.SetDataDir(t.TempDir())
	t.Cleanup(func() { model.SetDataDir("") })

	now := time.Date(2026, 8, 25, 15, 0, 0, 0, time.UTC)
	a, _ := subtaskReminderApp(t, srv.URL, now.Add(-30*time.Second), now)

	if got := a.subtaskReminders(); len(got) != 1 {
		t.Fatalf("collected %d task-line reminders, want 1: %+v", len(got), got)
	}
	runCmd(t, a.fireSubtaskReminders())

	got := c.got()
	if len(got) != 1 {
		t.Fatalf("pushes = %d, want 1: %v", len(got), got)
	}
	if !strings.Contains(got[0], "check zepto spider") {
		t.Errorf("push body = %q, want the subtask's text", got[0])
	}
}

// It must not fire again on the next sweep.
func TestSubtaskReminderFiresOnce(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(c.handler))
	defer srv.Close()

	model.SetDataDir(t.TempDir())
	t.Cleanup(func() { model.SetDataDir("") })

	now := time.Date(2026, 8, 25, 15, 0, 0, 0, time.UTC)
	a, _ := subtaskReminderApp(t, srv.URL, now.Add(-30*time.Second), now)

	runCmd(t, a.fireSubtaskReminders())
	runCmd(t, a.fireSubtaskReminders())

	if got := c.got(); len(got) != 1 {
		t.Errorf("pushes = %d, want 1: %v", len(got), got)
	}
}

// A reminder still in the future must wait.
func TestSubtaskReminderWaitsForItsTime(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(c.handler))
	defer srv.Close()

	model.SetDataDir(t.TempDir())
	t.Cleanup(func() { model.SetDataDir("") })

	now := time.Date(2026, 8, 25, 15, 0, 0, 0, time.UTC)
	a, _ := subtaskReminderApp(t, srv.URL, now.Add(time.Hour), now)

	runCmd(t, a.fireSubtaskReminders())
	if got := c.got(); len(got) != 0 {
		t.Errorf("fired early: %v", got)
	}
}

// A completed subtask should not be nagged about.
func TestCompletedSubtaskDoesNotRemind(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(c.handler))
	defer srv.Close()

	model.SetDataDir(t.TempDir())
	t.Cleanup(func() { model.SetDataDir("") })

	now := time.Date(2026, 8, 25, 15, 0, 0, 0, time.UTC)
	a, root := subtaskReminderApp(t, srv.URL, now.Add(-30*time.Second), now)

	notePath := filepath.Join(root, "Tickets", "AAA-1 In Progress live.md")
	body, _ := os.ReadFile(notePath)
	done := strings.Replace(string(body), "- [ ] check zepto spider", "- [x] check zepto spider", 1)
	os.WriteFile(notePath, []byte(done), 0o644)

	idx, _ := para.Scan(root, time.UTC)
	m, _ := a.Update(indexMsg{idx: idx})
	a = m.(App)

	runCmd(t, a.fireSubtaskReminders())
	if got := c.got(); len(got) != 0 {
		t.Errorf("reminded about a finished subtask: %v", got)
	}
}

// The tick has to drive this sweep, not just the other two.
func TestTickSweepsSubtaskReminders(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(c.handler))
	defer srv.Close()

	model.SetDataDir(t.TempDir())
	t.Cleanup(func() { model.SetDataDir("") })

	now := time.Date(2026, 8, 25, 15, 0, 0, 0, time.UTC)
	a, _ := subtaskReminderApp(t, srv.URL, now.Add(-30*time.Second), now)

	_, cmd := a.Update(reminderTickMsg(now))
	runCmd(t, cmd)

	if got := c.got(); len(got) != 1 {
		t.Errorf("pushes = %d, want the tick to have swept task lines: %v", len(got), got)
	}
}

// Identity is the note and the text, not the line number: notes get rewritten
// and a task moves up and down the file without its reminder changing.
func TestSubtaskReminderSurvivesTheLineMoving(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(c.handler))
	defer srv.Close()

	model.SetDataDir(t.TempDir())
	t.Cleanup(func() { model.SetDataDir("") })

	now := time.Date(2026, 8, 25, 15, 0, 0, 0, time.UTC)
	a, root := subtaskReminderApp(t, srv.URL, now.Add(-30*time.Second), now)
	runCmd(t, a.fireSubtaskReminders())

	// Insert a task above it, shifting its line number.
	notePath := filepath.Join(root, "Tickets", "AAA-1 In Progress live.md")
	body, _ := os.ReadFile(notePath)
	moved := strings.Replace(string(body), "## Notes & Sub-tasks",
		"## Notes & Sub-tasks\n- [ ] something new", 1)
	os.WriteFile(notePath, []byte(moved), 0o644)

	idx, _ := para.Scan(root, time.UTC)
	m, _ := a.Update(indexMsg{idx: idx})
	a = m.(App)

	runCmd(t, a.fireSubtaskReminders())
	if got := c.got(); len(got) != 1 {
		t.Errorf("pushes = %d, want no repeat after the line moved: %v", len(got), got)
	}
}
