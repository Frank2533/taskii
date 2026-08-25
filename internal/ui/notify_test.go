package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"taskii/internal/model"
	"taskii/internal/remind"
)

// capture is a stand-in ntfy server.
type capture struct {
	mu   sync.Mutex
	body []string
}

func (c *capture) handler(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	c.body = append(c.body, string(b))
	c.mu.Unlock()
}

func (c *capture) got() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.body))
	copy(out, c.body)
	return out
}

// runCmd drains a tea.Cmd so the pushes it contains actually execute.
//
// Batches nest: announce returns a batch, and it is itself batched with the
// other reminders, so this has to recurse rather than run one level.
func runCmd(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return
	}
	var wg sync.WaitGroup
	for _, c := range batch {
		c := c
		wg.Add(1)
		go func() { defer wg.Done(); runCmd(t, c) }()
	}
	wg.Wait()
}

func eventApp(t *testing.T, srvURL string, start time.Time, now time.Time) App {
	t.Helper()
	model.SetDataDir(t.TempDir())
	t.Cleanup(func() { model.SetDataDir("") })

	a := NewApp(Options{Mock: true, Location: time.UTC})
	a.now = func() time.Time { return now }
	a.tasks = nil
	a.fired = remind.NewFired()
	a.events = []model.Event{{
		ID: "e1", Title: "standup", Start: start, End: start.Add(15 * time.Minute),
	}}
	a.pushEnabled = srvURL != ""
	a.ntfyServer = srvURL
	a.ntfyTopic = "test-topic"
	return a
}

func TestEventRemindersPushAtEachLead(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(c.handler))
	defer srv.Close()

	start := time.Date(2026, 8, 26, 9, 30, 0, 0, time.UTC)
	for _, lead := range remind.Leads {
		a := eventApp(t, srv.URL, start, start.Add(-lead))
		runCmd(t, a.fireEventReminders())
	}

	got := c.got()
	if len(got) != 3 {
		t.Fatalf("pushes = %d, want one per lead: %v", len(got), got)
	}
	for _, want := range []string{"15 minutes", "5 minutes", "1 minute"} {
		var found bool
		for _, g := range got {
			if strings.Contains(g, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("no push said %q: %v", want, got)
		}
	}
}

// Nothing may leave the machine while push is off.
func TestNothingIsPushedWhenDisabled(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(c.handler))
	defer srv.Close()

	start := time.Date(2026, 8, 26, 9, 30, 0, 0, time.UTC)
	a := eventApp(t, srv.URL, start, start.Add(-time.Minute))
	a.pushEnabled = false
	runCmd(t, a.fireEventReminders())

	if got := c.got(); len(got) != 0 {
		t.Errorf("pushed while disabled: %v", got)
	}
}

func TestNothingIsPushedWithoutATopic(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(c.handler))
	defer srv.Close()

	start := time.Date(2026, 8, 26, 9, 30, 0, 0, time.UTC)
	a := eventApp(t, srv.URL, start, start.Add(-time.Minute))
	a.ntfyTopic = ""
	runCmd(t, a.fireEventReminders())

	if got := c.got(); len(got) != 0 {
		t.Errorf("pushed without a topic: %v", got)
	}
}

// A reminder must not go out twice for the same occurrence.
func TestEventReminderDoesNotRepeat(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(c.handler))
	defer srv.Close()

	start := time.Date(2026, 8, 26, 9, 30, 0, 0, time.UTC)
	a := eventApp(t, srv.URL, start, start.Add(-time.Minute))
	runCmd(t, a.fireEventReminders())
	runCmd(t, a.fireEventReminders())

	if got := c.got(); len(got) != 1 {
		t.Errorf("pushes = %d, want 1: %v", len(got), got)
	}
}

// The topic is a secret in all but name and must not be shown in full.
func TestSettingsMasksTheTopic(t *testing.T) {
	a := NewApp(Options{Mock: true, Location: time.UTC})
	a.width, a.height = 150, 44
	a.ntfyTopic = "taskii-8f3ka92mzq"
	a.settingsUI.open = true

	out := a.renderSettings()
	if strings.Contains(out, "8f3ka92mzq") {
		t.Errorf("the topic is shown in full:\n%s", out)
	}
	if !strings.Contains(out, "•") {
		t.Errorf("the topic is not masked:\n%s", out)
	}
}
