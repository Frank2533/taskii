package ui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// This exercises the literal key sequence the user describes: add a local
// task from the dashboard, switch to PARA, press r, and follow the command it
// returns all the way through Update — not just fabricate an indexMsg the way
// the other sync tests do, which would not catch a bug in the r key's own
// wiring.
func TestPressingRInParaPicksUpANewLocalTask(t *testing.T) {
	a, _ := syncedApp(t)

	a = press(t, a, "a")
	a = typeInto(a, "buy oat milk")
	a = press(t, a, "enter")
	if a.err != "" {
		t.Fatalf("add reported: %s", a.err)
	}

	a = press(t, a, "2") // PARA view
	next, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	a = next.(App)
	if cmd == nil {
		t.Fatal("r produced no command")
	}
	final, _ := a.Update(cmd())
	a = final.(App)

	if a.idx == nil {
		t.Fatal("no index after reindexing")
	}
	if len(a.idx.OpenLocalTasks()) != 1 {
		t.Fatalf("OpenLocalTasks = %d, want 1: %+v", len(a.idx.OpenLocalTasks()), a.idx.OpenLocalTasks())
	}
	if !strings.Contains(a.View(), "Local tasks") {
		t.Errorf("the Local tasks row is not shown after reindex:\n%s", a.View())
	}
}

// The reverse: a local task's note edited directly in Obsidian — title
// changed, a subtask ticked off — must show up after r too, not just a
// brand-new note.
func TestPressingRInParaPicksUpEditsToALocalTasksNote(t *testing.T) {
	a, root := syncedApp(t)

	a = press(t, a, "a")
	a = typeInto(a, "buy oat milk")
	a = press(t, a, "enter")

	a = press(t, a, "2")
	next, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	a = next.(App)
	final, _ := a.Update(cmd())
	a = final.(App)
	if len(a.idx.LocalTasks) != 1 {
		t.Fatalf("local tasks after first index = %d, want 1", len(a.idx.LocalTasks))
	}
	notePath := a.idx.LocalTasks[0].Path

	// Simulate an edit made directly in Obsidian: append a subtask line.
	body := readFile(t, notePath)
	edited := strings.Replace(body, "## Subtasks\n", "## Subtasks\n- [ ] find oat milk\n", 1)
	writeFile(t, notePath, edited)

	next, cmd = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	a = next.(App)
	final, _ = a.Update(cmd())
	a = final.(App)

	if len(a.idx.LocalTasks) != 1 || len(a.idx.LocalTasks[0].Checkboxes) != 1 {
		t.Fatalf("edit was not picked up: %+v", a.idx.LocalTasks)
	}
	_ = root
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// This is the actual root cause behind "reindexing makes no changes to local
// tasks": the Local tasks row only appears once there is at least one, the
// same way Unfiled does, and inserting it shifts every row after it down by
// one. treeSel was a plain position, so the cursor silently drifted onto
// whatever row now sat at its old index — the new Local tasks row existed,
// but nothing ever put the cursor on it, and whatever the cursor landed on
// instead often looked unchanged, which is exactly what "nothing happened"
// looks like from the outside.
func TestReindexKeepsTheTreeSelectionOnTheSameRowAfterInsertion(t *testing.T) {
	a, _ := syncedApp(t)
	a = press(t, a, "2") // PARA view, tree pane focused by default

	// Land on the QCOM area row, which sits after where a Local tasks row
	// would be inserted.
	var onQCOM App
	for i := 0; i < 10; i++ {
		if row := a.treeRows()[a.vault.treeSel]; row.kind == rowArea && row.area == "QCOM" {
			onQCOM = a
			break
		}
		a = press(t, a, "down")
	}
	if onQCOM.vault.treeKey == (treeRowKey{}) {
		t.Fatal("test setup: never found the QCOM row")
	}
	a = onQCOM
	beforeLabel := a.treeRows()[a.vault.treeSel].label
	if beforeLabel != "QCOM" {
		t.Fatalf("test setup: selected %q, want QCOM", beforeLabel)
	}

	// Add a local task (which is what makes a brand new Local tasks row
	// appear) and reindex through the real key.
	a = press(t, a, "1") // dashboard
	a = press(t, a, "a")
	a = typeInto(a, "buy oat milk")
	a = press(t, a, "enter")
	a = press(t, a, "2") // back to PARA

	next, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	a = next.(App)
	final, _ := a.Update(cmd())
	a = final.(App)

	rows := a.treeRows()
	if a.vault.treeSel < 0 || a.vault.treeSel >= len(rows) {
		t.Fatalf("selection out of range: %d of %d", a.vault.treeSel, len(rows))
	}
	afterLabel := rows[a.vault.treeSel].label
	if afterLabel != "QCOM" {
		t.Errorf("selection drifted from QCOM to %q after a Local tasks row was inserted", afterLabel)
	}

	// And the new row must genuinely be reachable — the bug isn't fixed by
	// coincidence if it turned out there was nothing to land on in the
	// first place.
	var sawLocalTasks bool
	for _, r := range rows {
		if r.kind == rowLocalTasks {
			sawLocalTasks = true
		}
	}
	if !sawLocalTasks {
		t.Fatal("test setup: no Local tasks row appeared at all")
	}
}
