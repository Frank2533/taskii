package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func note(id, title string) TaskNote {
	return TaskNote{ID: id, Title: title, Created: time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)}
}

func TestWriteTaskNoteCreatesAReadableNote(t *testing.T) {
	dir := t.TempDir()
	n := note("abc", "buy oat milk")
	n.Notes = []string{"the corner shop stocks it"}

	path, err := WriteTaskNote(dir, n)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "buy oat milk.md" {
		t.Errorf("name = %q, want the title", filepath.Base(path))
	}
	body, _ := os.ReadFile(path)
	for _, want := range []string{"taskii_id: abc", "type: task", "status: open", "## Subtasks", "## Notes", "the corner shop stocks it"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("note missing %q:\n%s", want, body)
		}
	}
}

// Renaming a task must move its note, not leave a second copy behind.
func TestRenamingATaskMovesItsNote(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteTaskNote(dir, note("abc", "buy milk")); err != nil {
		t.Fatal(err)
	}
	path, err := WriteTaskNote(dir, note("abc", "buy oat milk"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "buy oat milk.md" {
		t.Errorf("name = %q", filepath.Base(path))
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir holds %v, want just the renamed note", names)
	}
}

// Subtasks are edited through the note, so rewriting must not discard what was
// ticked off in Obsidian since taskii last wrote.
func TestSubtasksInTheNoteSurviveARewrite(t *testing.T) {
	dir := t.TempDir()
	n := note("abc", "ship the thing")
	n.Subtasks = []string{"- [ ] write it"}
	path, err := WriteTaskNote(dir, n)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetCheckbox(path, findLine(t, path, "write it"), "write it", true); err != nil {
		t.Fatal(err)
	}
	if err := AppendCheckbox(path, SubtasksHeading, "test it"); err != nil {
		t.Fatal(err)
	}

	// taskii rewrites the note with its own (stale) idea of the subtasks.
	if _, err := WriteTaskNote(dir, n); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "- [x] write it") {
		t.Errorf("a completed subtask was reverted:\n%s", body)
	}
	if !strings.Contains(string(body), "- [ ] test it") {
		t.Errorf("a subtask added in the vault was lost:\n%s", body)
	}
}

func findLine(t *testing.T, path, text string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, text) {
			return i
		}
	}
	t.Fatalf("no line containing %q", text)
	return -1
}

func TestArchiveMovesTheNote(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "Archive", "Local Tasks")
	path, err := WriteTaskNote(dir, note("abc", "done thing"))
	if err != nil {
		t.Fatal(err)
	}
	moved, err := ArchiveTaskNote(path, archive)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(moved) != archive {
		t.Errorf("moved to %q, want the archive", filepath.Dir(moved))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the original note is still there")
	}
}

// An existing name in the archive must not be overwritten.
func TestArchiveDoesNotClobber(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "Archive", "Local Tasks")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archive, "done thing.md"), []byte("older\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, _ := WriteTaskNote(dir, note("abc", "done thing"))
	moved, err := ArchiveTaskNote(path, archive)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(moved) == "done thing.md" {
		t.Error("the existing archived note was replaced")
	}
	older, _ := os.ReadFile(filepath.Join(archive, "done thing.md"))
	if string(older) != "older\n" {
		t.Error("the existing archived note was modified")
	}
}

func TestDailyNoteRewritesTheBoard(t *testing.T) {
	dir := t.TempDir()
	day := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)

	path, err := WriteDailyNote(dir, day, []string{"first", "second\nwith a continuation"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "2026-08-25.md" {
		t.Errorf("name = %q, want the date", filepath.Base(path))
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "- first") || !strings.Contains(string(body), "  with a continuation") {
		t.Errorf("board not rendered:\n%s", body)
	}

	// A note removed from the board must not linger in the vault.
	if _, err := WriteDailyNote(dir, day, []string{"first"}); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(path)
	if strings.Contains(string(body), "second") {
		t.Errorf("a deleted note survived the rewrite:\n%s", body)
	}
}

// The vault is watched and committed, so an unchanged board must not touch
// the file.
func TestDailyNoteSkipsIdenticalContent(t *testing.T) {
	dir := t.TempDir()
	day := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	path, _ := WriteDailyNote(dir, day, []string{"first"})
	info, _ := os.Stat(path)
	before := info.ModTime()

	if _, err := WriteDailyNote(dir, day, []string{"first"}); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(path)
	if !info.ModTime().Equal(before) {
		t.Error("identical content was rewritten")
	}
}

func TestFileNameIsSafe(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"fix a/b: the thing?", "fix a-b- the thing-.md"},
		{"   ", "task.md"},
		{"normal title", "normal title.md"},
	} {
		if got := FileName(c.in); got != c.want {
			t.Errorf("FileName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
