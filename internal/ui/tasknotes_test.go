package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"taskii/internal/para"
)

// syncedApp is a dashboard app that writes to the fixture vault.
func syncedApp(t *testing.T) (App, string) {
	t.Helper()
	a, root := dashApp(t)
	a.noPersist = false
	a.obsidianSync = true
	a.projectFolder = "Projects"
	return a, root
}

func reindex(t *testing.T, a App, root string) App {
	t.Helper()
	idx, err := para.Scan(root, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := a.Update(indexMsg{idx: idx})
	return m.(App)
}

// A note about one subtask belongs attached to that line, not in a section
// shared by the whole note.
func TestSubtaskNoteIsWrittenUnderItsLine(t *testing.T) {
	a, root := syncedApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "normalize")
	a = press(t, a, "down", "enter")

	a.todaySelected = 2 // the second subtask
	a = press(t, a, "N")
	if a.mode != modeTaskNote {
		t.Fatalf("mode = %v, want modeTaskNote", a.mode)
	}
	a = typeInto(a, "waiting on the DS team")
	a = press(t, a, "enter")
	if a.err != "" {
		t.Fatalf("note reported: %s", a.err)
	}

	body, err := os.ReadFile(filepath.Join(root, "Tickets", "AAA-1 In Progress live.md"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(body), "\n")
	for i, l := range lines {
		if strings.Contains(l, "- [ ] check zepto spider") {
			if !strings.HasPrefix(lines[i+1], "  ") || !strings.Contains(lines[i+1], "waiting on the DS team") {
				t.Fatalf("the note is not attached under the subtask:\n%s", body)
			}
			return
		}
	}
	t.Fatalf("the subtask line vanished:\n%s", body)
}

// A note about a ticket goes to its work log, the one section the Jira sync
// does not rewrite.
func TestTicketNoteGoesToTheWorkLog(t *testing.T) {
	a, root := syncedApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "normalize")
	a = press(t, a, "down", "enter")

	a.todaySelected = 0 // the ticket row
	a = press(t, a, "N")
	a = typeInto(a, "raised with the client")
	a = press(t, a, "enter")
	if a.err != "" {
		t.Fatalf("note reported: %s", a.err)
	}

	body, _ := os.ReadFile(filepath.Join(root, "Tickets", "AAA-1 In Progress live.md"))
	idx := strings.Index(string(body), "## Work Log / Updates")
	at := strings.Index(string(body), "raised with the client")
	if idx < 0 || at < idx {
		t.Errorf("the note did not land in the work log:\n%s", body)
	}
}

// A note about a local task goes into the body of its own note.
func TestLocalTaskNoteGoesIntoItsNoteBody(t *testing.T) {
	a, root := syncedApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "buy oat milk")
	a = press(t, a, "enter")

	a.todaySelected = 0
	a = press(t, a, "N")
	a = typeInto(a, "the corner shop stocks it")
	a = press(t, a, "enter")
	if a.err != "" {
		t.Fatalf("note reported: %s", a.err)
	}

	body, err := os.ReadFile(filepath.Join(root, "Projects", "buy oat milk.md"))
	if err != nil {
		t.Fatal(err)
	}
	idx := strings.Index(string(body), "## Notes")
	at := strings.Index(string(body), "the corner shop stocks it")
	if idx < 0 || at < idx {
		t.Errorf("the note did not land in the task's note body:\n%s", body)
	}
}

// The pane follows the cursor while a task list has focus, and returns to the
// day's board when focus moves away.
func TestNotesPaneFollowsTheSelectionAndComesBack(t *testing.T) {
	a, root := syncedApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "buy oat milk")
	a = press(t, a, "enter")
	a.todaySelected = 0
	a = press(t, a, "N")
	a = typeInto(a, "the corner shop stocks it")
	a = press(t, a, "enter")
	a = reindex(t, a, root)

	a.saveNote("today's board note")

	// Focus is on Today: the pane shows the task's notes.
	a.focus = focusToday
	out := a.View()
	if !strings.Contains(out, "Notes — buy oat milk") {
		t.Errorf("the pane is not following the selection:\n%s", out)
	}
	if !strings.Contains(out, "the corner shop stocks it") {
		t.Errorf("the task's note is not shown:\n%s", out)
	}
	if strings.Contains(out, "today's board note") {
		t.Errorf("the day's board is showing while a task is selected:\n%s", out)
	}

	// Move focus off the task lists: the board comes back.
	a.focus = focusNotes
	out = a.View()
	if !strings.Contains(out, "today's board note") {
		t.Errorf("the day's board did not come back:\n%s", out)
	}
	if strings.Contains(out, "Notes — buy oat milk") {
		t.Errorf("the pane is still following the selection:\n%s", out)
	}
}

// Notes on a task need somewhere in the vault to live, and saying so beats
// silently discarding them.
func TestTaskNotesRefusedWhenSyncIsOff(t *testing.T) {
	a, _ := dashApp(t)
	a.obsidianSync = false
	a = press(t, a, "a")
	a = typeInto(a, "buy oat milk")
	a = press(t, a, "enter")

	a.todaySelected = 0
	a = press(t, a, "N")
	if a.mode == modeTaskNote {
		t.Fatal("started a note with nowhere to put it")
	}
	if a.err == "" {
		t.Error("no explanation was given")
	}
	if !strings.Contains(a.View(), "not synced") {
		t.Errorf("the pane does not say the task is unsynced:\n%s", a.View())
	}
}

func TestTaskNoteCanBeCancelled(t *testing.T) {
	a, _ := syncedApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "buy oat milk")
	a = press(t, a, "enter")
	a.todaySelected = 0

	a = press(t, a, "N")
	a = typeInto(a, "never mind")
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	a = m.(App)

	if a.mode == modeTaskNote {
		t.Error("esc did not close the input")
	}
	if strings.Contains(a.View(), "never mind") {
		t.Error("the cancelled text is still shown")
	}
}

// A URL pasted into a task note must render clickable, in the pane where it
// actually appears — not just be provable at the helper-function level.
func TestURLInATaskNoteIsClickableInTheNotesPane(t *testing.T) {
	a, root := syncedApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "buy oat milk")
	a = press(t, a, "enter")

	a.todaySelected = 0
	a = press(t, a, "N")
	a = typeInto(a, "see https://example.com/receipt for the order")
	a = press(t, a, "enter")
	a = reindex(t, a, root)

	a.focus = focusToday
	out := a.View()
	if !strings.Contains(out, ansi.SetHyperlink("https://example.com/receipt")) {
		t.Errorf("the pasted URL is not clickable:\n%s", out)
	}
}
