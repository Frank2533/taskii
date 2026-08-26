package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// overdueApp is a dashboard app with one carried-over ticket task (yesterday's
// date, no deadline miss involved) already sitting in Overdue, and Obsidian
// sync on so vault-writing actions can be exercised.
func overdueApp(t *testing.T) (App, string) {
	t.Helper()
	a, root := syncedApp(t)

	a = press(t, a, "a")
	a = typeInto(a, "normalize")
	a = press(t, a, "down", "enter") // attach AAA-1, which has two subtasks
	if len(a.tasks) != 1 {
		t.Fatalf("setup: tasks = %d, want 1", len(a.tasks))
	}
	yesterday := a.now().AddDate(0, 0, -1).Format(dateFormat)
	a.tasks[0].Date = yesterday
	a.persist()
	a.focus = focusOverdue

	if len(a.overdueTasks()) != 1 {
		t.Fatalf("setup: overdue = %d, want 1", len(a.overdueTasks()))
	}
	return a, root
}

func TestOverdueExpandsATicketsSubtasks(t *testing.T) {
	a, _ := overdueApp(t)
	rows := a.overdueRows()
	if len(rows) != 3 {
		t.Fatalf("overdue rows = %d, want the ticket plus its 2 subtasks: %+v", len(rows), rows)
	}
	if !strings.Contains(a.View(), "check zepto spider") {
		t.Errorf("a carried-over ticket's subtasks are not shown in Overdue:\n%s", a.View())
	}
}

func TestOverdueToggleWritesSubtaskToTheVault(t *testing.T) {
	a, root := overdueApp(t)
	a.overdueSelected = 2 // the second subtask row

	next, cmd := a.Update(tea.KeyMsg{Type: tea.KeySpace})
	a = next.(App)
	if a.err != "" {
		t.Fatalf("toggle reported: %s", a.err)
	}
	if cmd == nil {
		t.Fatal("toggling a subtask in Overdue did not reindex")
	}

	body, err := os.ReadFile(filepath.Join(root, "Tickets", "AAA-1 In Progress live.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "- [x] check zepto spider") {
		t.Errorf("the subtask was not toggled in the vault:\n%s", body)
	}
}

func TestOverdueFoldCollapsesSubtasks(t *testing.T) {
	a, _ := overdueApp(t)
	a.overdueSelected = 0 // the ticket row
	a = press(t, a, "z")
	if got := len(a.overdueRows()); got != 1 {
		t.Fatalf("rows after fold = %d, want 1", got)
	}
	a = press(t, a, "z")
	if got := len(a.overdueRows()); got != 3 {
		t.Errorf("rows after unfold = %d, want 3", got)
	}
}

func TestOverdueAddSubtaskAppendsToTheVault(t *testing.T) {
	a, root := overdueApp(t)
	a.overdueSelected = 0
	a = press(t, a, "A")
	if a.mode != modeAddSubtask {
		t.Fatalf("mode = %v, want modeAddSubtask", a.mode)
	}
	if a.editingFocus != focusOverdue {
		t.Errorf("editingFocus = %v, want focusOverdue", a.editingFocus)
	}
	// The input must be drawn under Overdue, not silently glued under Today.
	out := a.View()
	overdueIdx := strings.Index(out, "Overdue")
	inputIdx := strings.Index(out, "new subtask for AAA-1")
	if overdueIdx < 0 || inputIdx < overdueIdx {
		t.Errorf("the add-subtask input is not shown under the Overdue pane:\n%s", out)
	}

	a = typeInto(a, "verify replication")
	a = press(t, a, "enter")
	if a.err != "" {
		t.Fatalf("add subtask reported: %s", a.err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "Tickets", "AAA-1 In Progress live.md"))
	if !strings.Contains(string(body), "- [ ] verify replication") {
		t.Errorf("subtask not appended:\n%s", body)
	}
}

func TestOverdueEditRendersUnderOverdueNotToday(t *testing.T) {
	a, _ := overdueApp(t)
	a.overdueSelected = 0
	a = press(t, a, "e")
	if a.mode != modeEditRow || a.editingFocus != focusOverdue {
		t.Fatalf("mode=%v editingFocus=%v, want modeEditRow/focusOverdue", a.mode, a.editingFocus)
	}
	out := a.View()
	overdueIdx := strings.Index(out, "Overdue")
	editIdx := strings.LastIndex(out, "AAA-1")
	if overdueIdx < 0 || editIdx < overdueIdx {
		t.Errorf("the edit input does not appear to be under Overdue:\n%s", out)
	}
}

func TestOverdueDeadlinePickerSetsASubtaskReminder(t *testing.T) {
	a, root := overdueApp(t)
	a.overdueSelected = 2 // second subtask
	a = press(t, a, "D")
	if !a.picker.open {
		t.Fatal("picker did not open in Overdue")
	}
	if !strings.Contains(a.View(), "check zepto spider") {
		t.Errorf("picker does not name the selected subtask:\n%s", a.View())
	}
	next, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")}) // tomorrow
	a = next.(App)
	if cmd != nil {
		final, _ := a.Update(cmd())
		a = final.(App)
	}
	if a.err != "" {
		t.Fatalf("picker reported: %s", a.err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "Tickets", "AAA-1 In Progress live.md"))
	if !strings.Contains(string(body), "📅") {
		t.Errorf("no deadline written from the picker in Overdue:\n%s", body)
	}
}

func TestOverdueTaskNoteWorksOnASubtask(t *testing.T) {
	a, root := overdueApp(t)
	a.overdueSelected = 2
	a = press(t, a, "N")
	if a.mode != modeTaskNote {
		t.Fatalf("mode = %v, want modeTaskNote", a.mode)
	}
	a = typeInto(a, "waiting on DS team")
	a = press(t, a, "enter")
	if a.err != "" {
		t.Fatalf("note reported: %s", a.err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "Tickets", "AAA-1 In Progress live.md"))
	if !strings.Contains(string(body), "waiting on DS team") {
		t.Errorf("note not written from Overdue:\n%s", body)
	}
}

// The one genuinely new feature: an undo for a task that became overdue
// purely by being carried over.
func TestBumpToTodayMovesTheTaskOut(t *testing.T) {
	a, _ := overdueApp(t)
	a.overdueSelected = 0
	before := a.tasks[0].Date

	a = press(t, a, "T")
	if a.err != "" {
		t.Fatalf("T reported: %s", a.err)
	}
	if a.tasks[0].Date == before {
		t.Fatal("the task's Date was not updated")
	}
	if len(a.overdueTasks()) != 0 {
		t.Errorf("still %d task(s) in Overdue after bumping", len(a.overdueTasks()))
	}
	if len(a.todayTasks()) != 1 {
		t.Errorf("the task did not reappear in Today")
	}
}

// A subtask has no Date of its own, so T on one must explain itself rather
// than silently doing nothing or bumping the wrong thing.
func TestBumpToTodayRefusesOnASubtask(t *testing.T) {
	a, _ := overdueApp(t)
	a.overdueSelected = 2
	before := a.tasks[0].Date

	a = press(t, a, "T")
	if a.err == "" {
		t.Error("no explanation given for T on a subtask")
	}
	if a.tasks[0].Date != before {
		t.Error("the ticket's Date was changed despite the cursor being on a subtask")
	}
}

// A task whose deadline was missed keeps that deadline after being bumped —
// bumping only changes which day it is filed under, not the deadline machinery.
func TestBumpToTodayDoesNotClearAMissedDeadline(t *testing.T) {
	a, _ := syncedApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "buy oat milk")
	a = press(t, a, "enter")
	past := a.now().AddDate(0, 0, -3)
	a.tasks[0].Due = &past
	a.tasks[0].Date = past.Format(dateFormat)
	a.focus = focusOverdue
	a.overdueSelected = 0

	a = press(t, a, "T")
	if a.tasks[0].Due == nil || !a.tasks[0].Due.Equal(past) {
		t.Error("bumping to today should not touch an existing deadline")
	}
	// Still overdue by deadline, now also filed under today.
	if len(a.overdueTasks()) != 1 {
		t.Error("a task with a still-missed deadline should still show in Overdue")
	}
	if len(a.todayTasks()) != 1 {
		t.Error("the task should also now show in Today")
	}
}
