package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"taskii/internal/model"
)

// Typing a deadline token must set the deadline and keep it out of the title.
func TestAddTaskParsesDeadlineTokens(t *testing.T) {
	a := NewApp(Options{Mock: true, Location: time.UTC})
	m, _ := a.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	app := m.(App)
	app.addTask("fix the spider !tmr @3h")

	var found bool
	for _, task := range app.tasks {
		if task.Title != "fix the spider" {
			continue
		}
		found = true
		if !task.HasDue() {
			t.Error("no deadline recorded")
		}
		if task.RemindAt == nil {
			t.Error("no reminder recorded")
		}
	}
	if !found {
		t.Fatalf("task not added with a clean title; got %+v", titles(app))
	}
}

func titles(a App) []string {
	var out []string
	for _, t := range a.tasks {
		out = append(out, t.Title)
	}
	return out
}

// A missed deadline is late even when the task is filed under today, and it
// must outrank a task merely carried over from an earlier day.
func TestOverdueIncludesMissedDeadlinesFirst(t *testing.T) {
	now := time.Date(2026, 8, 26, 15, 0, 0, 0, time.UTC)
	a := NewApp(Options{Mock: true, Location: time.UTC})
	a.now = func() time.Time { return now }
	a.tasks = nil

	yesterday := now.AddDate(0, 0, -1)
	missed := now.Add(-2 * time.Hour)
	a.tasks = append(a.tasks,
		taskAt("carried over", yesterday.Format(dateFormat), nil),
		taskAt("missed deadline", now.Format(dateFormat), &missed),
	)

	got := a.overdueTasks()
	if len(got) != 2 {
		t.Fatalf("overdue = %d rows, want 2: %+v", len(got), got)
	}
	if got[0].Title != "missed deadline" {
		t.Errorf("first row = %q, want the missed deadline", got[0].Title)
	}
}

// A task due at the end of today is not late at midday.
func TestDueTodayIsNotYetOverdue(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	endOfDay := time.Date(2026, 8, 26, 23, 59, 59, 0, time.UTC)
	a := NewApp(Options{Mock: true, Location: time.UTC})
	a.now = func() time.Time { return now }
	a.tasks = []model.Task{}[:0]
	a.tasks = append(a.tasks, taskAt("due tonight", now.Format(dateFormat), &endOfDay))

	if len(a.overdueTasks()) != 0 {
		t.Error("a task due at end of today should not be overdue at midday")
	}
}

func TestDeadlineBadgeAppearsInTheRow(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	due := time.Date(2026, 8, 27, 23, 59, 59, 0, time.UTC)
	rows := decorateDeadlines([]model.Task{taskAt("normalize cities", now.Format(dateFormat), &due)}, now)
	if !strings.Contains(rows[0].Title, "tomorrow") {
		t.Errorf("title = %q, want a deadline marker", rows[0].Title)
	}
}

// Decoration must never reach the stored list, or markers would accumulate on
// every save.
func TestDecorationDoesNotMutateStoredTasks(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	due := now.AddDate(0, 0, 1)
	original := []model.Task{taskAt("normalize cities", now.Format(dateFormat), &due)}
	decorateDeadlines(original, now)
	decorateDeadlines(original, now)
	if original[0].Title != "normalize cities" {
		t.Errorf("stored title = %q, want it untouched", original[0].Title)
	}
}

func taskAt(title, date string, due *time.Time) model.Task {
	t := model.Task{ID: title, Title: title, Date: date, CreatedAt: time.Now()}
	t.Due = due
	return t
}

func TestTimelinePlacesRemindersAndDeadlines(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	a := NewApp(Options{Mock: true, Location: time.UTC})
	a.now = func() time.Time { return now }
	a.width, a.height = 150, 40

	remind := time.Date(2026, 8, 26, 15, 0, 0, 0, time.UTC)
	due := time.Date(2026, 8, 26, 23, 59, 59, 0, time.UTC)
	task := taskAt("zepto spider", now.Format(dateFormat), &due)
	task.RemindAt = &remind
	a.tasks = []model.Task{task, taskAt("no time set", now.Format(dateFormat), nil)}

	items, anytime := a.timelineItems()
	if len(items) != 2 {
		t.Fatalf("timeline items = %d, want a reminder and a deadline: %+v", len(items), items)
	}
	if items[0].kind != tlReminder || items[1].kind != tlDeadline {
		t.Errorf("order = %v/%v, want reminder then deadline", items[0].kind, items[1].kind)
	}
	if anytime != 1 {
		t.Errorf("anytime = %d, want the untimed task counted separately", anytime)
	}

	out := a.renderTimelinePane(60, 12)
	for _, want := range []string{"Today", "start: zepto spider", "due: zepto spider", "now 12:00"} {
		if !strings.Contains(out, want) {
			t.Errorf("timeline missing %q:\n%s", want, out)
		}
	}
}

// A reminder must fire once and stay fired, or restarting replays every
// reminder that already passed.
func TestRemindersFireOnce(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)

	early := taskAt("due now", now.Format(dateFormat), nil)
	early.RemindAt = &past
	later := taskAt("later", now.Format(dateFormat), nil)
	later.RemindAt = &future

	tasks := []model.Task{early, later}
	got := dueReminders(tasks, now)
	if len(got) != 1 || tasks[got[0]].Title != "due now" {
		t.Fatalf("dueReminders = %v, want just the past one", got)
	}
	tasks[got[0]].Reminded = true
	if len(dueReminders(tasks, now)) != 0 {
		t.Error("a fired reminder fired again")
	}
}

// The timeline lives on the dashboard, in height taken from the Notes board.
func TestTimelineAppearsOnTheDashboard(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	a := NewApp(Options{Mock: true, Location: time.UTC})
	a.now = func() time.Time { return now }
	a.tasks = nil
	m, _ := a.Update(tea.WindowSizeMsg{Width: 150, Height: 44})
	app := m.(App)

	g := app.geometry()
	if g.timelineHeight == 0 {
		t.Fatal("no timeline height was allocated on a tall terminal")
	}
	if g.notesHeight < notesMinContentLines+2 {
		t.Errorf("Notes was squeezed below its minimum: %d", g.notesHeight)
	}
	if !strings.Contains(app.View(), "Today — ") {
		t.Errorf("timeline pane is not on the dashboard:\n%s", app.View())
	}
}

// A short terminal keeps the board rather than rendering two useless slivers.
func TestTimelineYieldsToNotesWhenThereIsNoRoom(t *testing.T) {
	a := NewApp(Options{Mock: true, Location: time.UTC})
	a.tasks = nil
	m, _ := a.Update(tea.WindowSizeMsg{Width: 150, Height: 24})
	app := m.(App)
	g := app.geometry()
	if g.notesHeight > 0 && g.notesHeight < notesMinContentLines+2 {
		t.Errorf("Notes = %d, below its minimum", g.notesHeight)
	}
}

// Editing a deadline must show up immediately: the timeline is derived, not
// cached.
func TestTimelineReflectsEditsImmediately(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	a := NewApp(Options{Mock: true, Location: time.UTC})
	a.now = func() time.Time { return now }
	a.tasks = nil
	m, _ := a.Update(tea.WindowSizeMsg{Width: 150, Height: 44})
	app := m.(App)

	if strings.Contains(app.View(), "ship the release") {
		t.Fatal("task is somehow already present")
	}
	app.addTask("ship the release !today")
	if !strings.Contains(app.View(), "due: ship the release") {
		t.Errorf("the timeline did not pick up the new deadline:\n%s", app.View())
	}
}
