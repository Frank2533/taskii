package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"taskii/internal/para"
)

// dashApp is a dashboard app backed by the fixture vault, with the index
// already delivered.
func dashApp(t *testing.T) (App, string) {
	t.Helper()
	root := fixtureVault(t)
	a := NewApp(Options{Mock: true, Vault: root, Location: time.UTC})
	a.loc = time.UTC
	// Mock seeds sample tasks for screenshots; clear them so row indices in
	// these tests refer only to what the test itself adds.
	a.tasks = nil
	a.notes = nil
	m, _ := a.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	idx, err := para.Scan(root, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.(App).Update(indexMsg{idx: idx})
	return m.(App), root
}

func typeInto(a App, s string) App {
	for _, r := range s {
		m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		a = m.(App)
	}
	return a
}

// Tickets must never appear in Today on their own; they get there only by
// being chosen.
func TestTodayDoesNotAutoPopulateTickets(t *testing.T) {
	a, _ := dashApp(t)
	if len(a.todayRows()) != 0 {
		t.Errorf("Today started with %d rows, want none", len(a.todayRows()))
	}
}

func TestTypeaheadOffersTicketsAsYouType(t *testing.T) {
	a, _ := dashApp(t)
	a = press(t, a, "a")
	if a.mode != modeAdding {
		t.Fatalf("mode = %v, want modeAdding", a.mode)
	}
	a = typeInto(a, "normalize")

	sugg := a.addSuggestions()
	if len(sugg) == 0 {
		t.Fatal("no suggestions for a matching summary")
	}
	if sugg[0].Key != "AAA-1" {
		t.Errorf("first suggestion = %q, want AAA-1", sugg[0].Key)
	}
	if !strings.Contains(a.View(), "normalize cities") {
		t.Error("suggestions are not rendered")
	}
}

// Enter with nothing highlighted adds a plain task, so non-ticket work stays
// first-class.
func TestPlainEnterAddsANonTicketTask(t *testing.T) {
	a, _ := dashApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "buy milk")
	a = press(t, a, "enter")

	rows := a.todayRows()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].task.Title != "buy milk" || rows[0].task.IsTicket() {
		t.Errorf("row = %+v, want a plain task", rows[0].task)
	}
}

// Selecting a suggestion attaches the row to that ticket and pulls its
// subtasks in beneath it.
func TestSelectingASuggestionAddsTicketWithSubtasks(t *testing.T) {
	a, _ := dashApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "normalize")
	a = press(t, a, "down")  // highlight the first suggestion
	a = press(t, a, "enter") // choose it

	rows := a.todayRows()
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want the ticket plus its 2 subtasks: %+v", len(rows), rows)
	}
	if rows[0].task.TicketKey != "AAA-1" {
		t.Errorf("ticket key = %q, want AAA-1", rows[0].task.TicketKey)
	}
	if !rows[1].isSub || !rows[2].isSub {
		t.Error("subtasks did not expand under the ticket")
	}
	if !strings.Contains(a.View(), "check zepto spider") {
		t.Error("subtask text is not rendered in the dashboard")
	}
}

func TestCollapseHidesSubtasksButKeepsTheCount(t *testing.T) {
	a, _ := dashApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "normalize")
	a = press(t, a, "down", "enter")

	a.todaySelected = 0
	a = press(t, a, "z")
	if got := len(a.todayRows()); got != 1 {
		t.Fatalf("rows after collapse = %d, want 1", got)
	}
	out := a.View()
	if !strings.Contains(out, "1/2") {
		t.Errorf("collapsed row should still report progress:\n%s", out)
	}
	a = press(t, a, "z")
	if got := len(a.todayRows()); got != 3 {
		t.Errorf("rows after expand = %d, want 3", got)
	}
}

// Ticking a subtask in the dashboard must write through to the vault note, so
// the note stays the single source of truth.
func TestTogglingASubtaskWritesToTheVault(t *testing.T) {
	a, root := dashApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "normalize")
	a = press(t, a, "down", "enter")

	notePath := filepath.Join(root, "Tickets", "AAA-1 In Progress live.md")
	before, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), "- [ ] check zepto spider") {
		t.Fatalf("fixture is not as expected:\n%s", before)
	}

	// Select the second subtask ("check zepto spider") and toggle it.
	a.todaySelected = 2
	next, cmd := a.Update(tea.KeyMsg{Type: tea.KeySpace})
	a = next.(App)
	if a.err != "" {
		t.Fatalf("toggle reported: %s", a.err)
	}

	after, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "- [x] check zepto spider") {
		t.Errorf("the note was not updated:\n%s", after)
	}
	if cmd == nil {
		t.Error("toggling should trigger a reindex so the two views agree")
	}
}
