package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const ticketNote = `---
key: AAA-1
summary: a ticket
status: In Progress
area: 
---

## Notes & Sub-tasks
- [x] done one
- [ ] open one

## Work Log / Updates
- 

## Related Notes
- 
`

func tempNote(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func lines(t *testing.T, path string) []string {
	t.Helper()
	return strings.Split(read(t, path), "\n")
}

func TestAppendCheckboxJoinsExistingList(t *testing.T) {
	path := tempNote(t, ticketNote)
	if err := AppendCheckbox(path, "Notes & Sub-tasks", "third thing"); err != nil {
		t.Fatal(err)
	}
	got := lines(t, path)
	// It must land directly after the last task, not after the blank line that
	// separates this section from the next.
	for i, l := range got {
		if l == "- [ ] open one" {
			if got[i+1] != "- [ ] third thing" {
				t.Fatalf("new task landed at %q, want right after the list", got[i+1])
			}
			return
		}
	}
	t.Fatal("existing task line vanished")
}

func TestAppendCheckboxCreatesMissingHeading(t *testing.T) {
	path := tempNote(t, "---\nkey: AAA-1\n---\n\n# Title\n")
	if err := AppendCheckbox(path, "Notes & Sub-tasks", "first"); err != nil {
		t.Fatal(err)
	}
	body := read(t, path)
	if !strings.Contains(body, "## Notes & Sub-tasks\n- [ ] first") {
		t.Fatalf("heading not created:\n%s", body)
	}
}

func TestAppendLeavesOtherSectionsAlone(t *testing.T) {
	path := tempNote(t, ticketNote)
	before := read(t, path)
	if err := AppendCheckbox(path, "Notes & Sub-tasks", "extra"); err != nil {
		t.Fatal(err)
	}
	after := read(t, path)
	for _, section := range []string{"## Work Log / Updates", "## Related Notes", "key: AAA-1"} {
		if strings.Count(before, section) != strings.Count(after, section) {
			t.Errorf("section %q was disturbed", section)
		}
	}
}

func TestSetCheckboxToggles(t *testing.T) {
	path := tempNote(t, ticketNote)
	if err := SetCheckbox(path, 9, "open one", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, path), "- [x] open one") {
		t.Errorf("not toggled:\n%s", read(t, path))
	}
}

// The index can describe a file the vault has already rewritten, so a toggle
// must refuse rather than flip whatever now sits on that line.
func TestSetCheckboxRefusesStaleLine(t *testing.T) {
	path := tempNote(t, ticketNote)
	err := SetCheckbox(path, 9, "some other task", true)
	if err == nil {
		t.Fatal("expected a staleness error")
	}
	var stale ErrStale
	if !asStale(err, &stale) {
		t.Fatalf("error = %v, want ErrStale", err)
	}
	if strings.Contains(read(t, path), "- [x] open one") {
		t.Error("file was modified despite the mismatch")
	}
}

func TestSetCheckboxRefusesNonTaskLine(t *testing.T) {
	path := tempNote(t, ticketNote)
	if err := SetCheckbox(path, 0, "", true); err == nil {
		t.Fatal("expected refusal on the frontmatter fence")
	}
}

func TestSetCheckboxIsIdempotent(t *testing.T) {
	path := tempNote(t, ticketNote)
	before := read(t, path)
	if err := SetCheckbox(path, 8, "done one", true); err != nil {
		t.Fatal(err)
	}
	if read(t, path) != before {
		t.Error("already-checked task was rewritten")
	}
}

func TestSetPropertyUpdatesExisting(t *testing.T) {
	path := tempNote(t, ticketNote)
	if err := SetProperty(path, "area", "QCOM"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, path), "area: QCOM") {
		t.Errorf("area not set:\n%s", read(t, path))
	}
	if strings.Count(read(t, path), "area:") != 1 {
		t.Error("area key duplicated")
	}
}

func TestSetPropertyAddsMissingKey(t *testing.T) {
	path := tempNote(t, ticketNote)
	if err := SetProperty(path, "jira_worklog_batch", "1h"); err != nil {
		t.Fatal(err)
	}
	body := read(t, path)
	if !strings.Contains(body, "jira_worklog_batch: 1h") {
		t.Errorf("key not added:\n%s", body)
	}
	// It must stay inside the frontmatter fence.
	idx := strings.Index(body, "jira_worklog_batch")
	closing := strings.Index(body[4:], "---")
	if idx > closing+4 {
		t.Error("key was written outside the frontmatter")
	}
}

func TestSetPropertyRefusesBlockList(t *testing.T) {
	path := tempNote(t, "---\nlabels:\n  - one\n  - two\n---\n")
	if err := SetProperty(path, "labels", "three"); err == nil {
		t.Fatal("expected refusal rather than mangling a list")
	}
}

func TestWriteIsAtomicAndPreservesMode(t *testing.T) {
	path := tempNote(t, ticketNote)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AppendCheckbox(path, "Notes & Sub-tasks", "x"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	// No temp files may survive next to the note.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".taskii-") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}

func asStale(err error, target *ErrStale) bool {
	if s, ok := err.(ErrStale); ok {
		*target = s
		return true
	}
	return false
}
