package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"taskii/internal/model"
	"taskii/internal/para"
)

// dashApp is a dashboard app backed by the fixture vault, with the index
// already delivered.
func dashApp(t *testing.T) (App, string) {
	t.Helper()
	root := fixtureVault(t)
	// Point persistence at a scratch directory: these tests exercise the
	// non-mock save path, and must never touch the developer's real data.
	model.SetDataDir(t.TempDir())
	t.Cleanup(func() { model.SetDataDir("") })
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

func TestTicketNoteAppendsToWorkLog(t *testing.T) {
	a, root := dashApp(t)
	a = press(t, a, "2")   // PARA view
	a = press(t, a, "tab") // focus the ticket list
	a = press(t, a, "n")   // start a note
	if a.mode != modeTicketNote {
		t.Fatalf("mode = %v, want modeTicketNote", a.mode)
	}
	a = typeInto(a, "spoke to DS team")
	if !strings.Contains(a.View(), "spoke to DS team") {
		t.Error("the note input is not rendered")
	}
	a = press(t, a, "enter")

	notePath := filepath.Join(root, "Tickets", "AAA-1 In Progress live.md")
	body, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "spoke to DS team") {
		t.Errorf("note was not written:\n%s", body)
	}
	// It must land in the freeform section, not in one the Jira sync rewrites.
	idx := strings.Index(string(body), "## Work Log / Updates")
	if idx < 0 || strings.Index(string(body), "spoke to DS team") < idx {
		t.Error("note did not land under Work Log / Updates")
	}
}

func TestDeadlinePickerSetsADeadline(t *testing.T) {
	a, _ := dashApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "buy milk")
	a = press(t, a, "enter")

	a.todaySelected = 0
	a = press(t, a, "D")
	if !a.picker.open {
		t.Fatal("picker did not open")
	}
	if !strings.Contains(a.View(), "tomorrow") {
		t.Error("picker does not list its options")
	}
	a = press(t, a, "m") // tomorrow
	if a.picker.open {
		t.Error("picker stayed open after a choice")
	}
	if len(a.tasks) != 1 || !a.tasks[0].HasDue() {
		t.Fatalf("no deadline set: %+v", a.tasks)
	}
	if got := a.tasks[0].Deadline().Day(); got != a.now().AddDate(0, 0, 1).Day() {
		t.Errorf("deadline day = %d, want tomorrow", got)
	}
}

func TestKeyReferenceOverlayListsBindings(t *testing.T) {
	a, _ := dashApp(t)
	a = press(t, a, "?")
	if !a.showKeys {
		t.Fatal("? did not open the reference")
	}
	out := a.View()
	for _, want := range []string{"add a subtask", "fold a ticket", "set a deadline", "!today"} {
		if !strings.Contains(out, want) {
			t.Errorf("reference missing %q", want)
		}
	}
	// Any key dismisses it, so it can never trap the user.
	a = press(t, a, "x")
	if a.showKeys {
		t.Error("reference stayed open")
	}
}

func TestEditingATaskChangesItsTitle(t *testing.T) {
	a, _ := dashApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "buy milk")
	a = press(t, a, "enter")

	a.todaySelected = 0
	a = press(t, a, "e")
	if a.mode != modeEditRow {
		t.Fatalf("mode = %v, want modeEditRow", a.mode)
	}
	if a.input.Value() != "buy milk" {
		t.Errorf("input seeded with %q, want the existing title", a.input.Value())
	}
	a.input.SetValue("buy oat milk !tmr")
	a = press(t, a, "enter")

	if a.tasks[0].Title != "buy oat milk" {
		t.Errorf("title = %q, want the edited text with tokens stripped", a.tasks[0].Title)
	}
	if !a.tasks[0].HasDue() {
		t.Error("editing should accept deadline tokens too")
	}
}

func TestEditingASubtaskWritesToTheVault(t *testing.T) {
	a, root := dashApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "normalize")
	a = press(t, a, "down", "enter")

	a.todaySelected = 2 // the second subtask
	a = press(t, a, "e")
	if a.mode != modeEditRow {
		t.Fatalf("mode = %v, want modeEditRow", a.mode)
	}
	a.input.SetValue("check zepto store spider")
	next, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a = next.(App)
	if a.err != "" {
		t.Fatalf("edit reported: %s", a.err)
	}
	if cmd == nil {
		t.Error("editing a subtask should reindex")
	}

	body, err := os.ReadFile(filepath.Join(root, "Tickets", "AAA-1 In Progress live.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "- [ ] check zepto store spider") {
		t.Errorf("subtask text not updated:\n%s", body)
	}
}

func TestAddSubtaskAppendsToTheTicketNote(t *testing.T) {
	a, root := dashApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "normalize")
	a = press(t, a, "down", "enter")

	a.todaySelected = 0 // the ticket row
	a = press(t, a, "A")
	if a.mode != modeAddSubtask {
		t.Fatalf("mode = %v, want modeAddSubtask", a.mode)
	}
	a.input.SetValue("verify replication")
	a = press(t, a, "enter")
	if a.err != "" {
		t.Fatalf("add subtask reported: %s", a.err)
	}

	body, err := os.ReadFile(filepath.Join(root, "Tickets", "AAA-1 In Progress live.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "- [ ] verify replication") {
		t.Errorf("subtask not appended:\n%s", body)
	}
}

// Finished work must never be offered as something to start today.
func TestSuggestionsExcludeArchivedAndClosedTickets(t *testing.T) {
	a, _ := dashApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "archived")
	for _, m := range a.addSuggestions() {
		if m.Key == "AAA-3" || m.Key == "AAA-4" {
			t.Errorf("archived ticket %s was suggested", m.Key)
		}
	}
	a = press(t, a, "esc")
	a = press(t, a, "a")
	a = typeInto(a, "stranded")
	for _, m := range a.addSuggestions() {
		if m.Key == "AAA-2" {
			t.Error("a closed ticket was suggested")
		}
	}
}

func TestSuggestionsShowTicketStatus(t *testing.T) {
	a, _ := dashApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "normalize")
	if !strings.Contains(a.View(), "[In Progress]") {
		t.Errorf("suggestion does not show status:\n%s", a.View())
	}
}

// With sync on, a local task becomes a note in the vault and shows up in the
// PARA view alongside the tickets.
func TestLocalTaskSyncsToVaultAndAppearsInPara(t *testing.T) {
	a, root := dashApp(t)
	a.noPersist = false // the sync path is skipped for mock runs
	a.obsidianSync = true
	a.projectFolder = "Projects"

	a = press(t, a, "a")
	a = typeInto(a, "buy oat milk")
	a = press(t, a, "enter")
	if a.err != "" {
		t.Fatalf("add reported: %s", a.err)
	}

	notePath := filepath.Join(root, "Projects", "buy oat milk.md")
	body, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("no note written: %v", err)
	}
	if !strings.Contains(string(body), "taskii_id:") {
		t.Errorf("note has no id:\n%s", body)
	}

	// It should now be indexed as a local task and visible in PARA.
	idx, err := para.Scan(root, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.OpenLocalTasks()) != 1 {
		t.Fatalf("local tasks = %d, want 1", len(idx.OpenLocalTasks()))
	}
	m, _ := a.Update(indexMsg{idx: idx})
	app := press(t, m.(App), "2")
	if !strings.Contains(app.View(), "Local tasks") {
		t.Errorf("PARA view has no local tasks row:\n%s", app.View())
	}
}

// Finishing a local task moves its note to the archive, which the vault's own
// mover would never do because its rules only watch the tickets folder.
func TestFinishingALocalTaskArchivesItsNote(t *testing.T) {
	a, root := dashApp(t)
	a.noPersist = false
	a.obsidianSync = true
	a.projectFolder = "Projects"

	a = press(t, a, "a")
	a = typeInto(a, "ship it")
	a = press(t, a, "enter")

	a.todaySelected = 0
	a = press(t, a, "space")
	if a.err != "" {
		t.Fatalf("toggle reported: %s", a.err)
	}

	if _, err := os.Stat(filepath.Join(root, "Projects", "ship it.md")); !os.IsNotExist(err) {
		t.Error("the note is still in the working folder")
	}
	if _, err := os.Stat(filepath.Join(root, "Archive", "Local Tasks", "ship it.md")); err != nil {
		t.Errorf("the note was not archived: %v", err)
	}
}

// Sync is opt-in: with it off, nothing may be written to the vault.
func TestNothingIsWrittenWhenSyncIsOff(t *testing.T) {
	a, root := dashApp(t)
	a.noPersist = false
	a.obsidianSync = false

	a = press(t, a, "a")
	a = typeInto(a, "buy oat milk")
	a = press(t, a, "enter")

	if _, err := os.Stat(filepath.Join(root, "Projects", "buy oat milk.md")); !os.IsNotExist(err) {
		t.Error("a note was written despite sync being disabled")
	}
}
