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
