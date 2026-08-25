package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// TaskNote is a local (non-ticket) task represented as a vault note.
type TaskNote struct {
	ID       string
	Title    string
	Done     bool
	Created  time.Time
	Due      *time.Time
	Subtasks []string
	Notes    []string
}

// Headings taskii owns inside a task note. Everything the user writes outside
// them is preserved on rewrite.
const (
	SubtasksHeading = "Subtasks"
	NotesHeading    = "Notes"
)

// unsafeFilename matches characters that are illegal or awkward in a filename
// on some platform, plus the leading dot that would hide the note.
var unsafeFilename = regexp.MustCompile(`[/\\:*?"<>|#^\[\]]+`)

// FileName turns a task title into a safe note name.
//
// Obsidian resolves links by filename, so this must be stable and readable
// rather than an opaque id.
func FileName(title string) string {
	name := unsafeFilename.ReplaceAllString(title, "-")
	name = strings.Trim(strings.Join(strings.Fields(name), " "), " .")
	if name == "" {
		name = "task"
	}
	// Leave room for the extension and for a filesystem that caps at 255.
	if len(name) > 120 {
		name = strings.TrimSpace(name[:120])
	}
	return name + ".md"
}

// renderTaskNote builds the note body.
func renderTaskNote(t TaskNote) string {
	status := "open"
	if t.Done {
		status = "done"
	}
	var b strings.Builder
	b.WriteString("---\n")
	// taskii_id is the join key. The filename follows the title and the note
	// can be moved between folders, so neither is a reliable identity.
	fmt.Fprintf(&b, "taskii_id: %s\n", t.ID)
	b.WriteString("type: task\n")
	fmt.Fprintf(&b, "status: %s\n", status)
	fmt.Fprintf(&b, "created: %s\n", t.Created.Format(time.RFC3339))
	if t.Due != nil {
		fmt.Fprintf(&b, "due: %s\n", t.Due.Format("2006-01-02"))
	}
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n\n", t.Title)

	fmt.Fprintf(&b, "## %s\n", SubtasksHeading)
	for _, s := range t.Subtasks {
		fmt.Fprintf(&b, "%s\n", s)
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## %s\n", NotesHeading)
	for _, n := range t.Notes {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	return b.String()
}

// WriteTaskNote creates or updates a task's note in dir and returns its path.
//
// An existing note is located by taskii_id rather than by name, so renaming a
// task moves its note instead of leaving a duplicate behind. Subtask lines
// already in the note are preserved: they are edited through the note itself,
// and rewriting them from taskii's copy would discard anything ticked off in
// Obsidian since.
func WriteTaskNote(dir string, t TaskNote) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(dir, FileName(t.Title))

	existing, err := FindNoteByID(dir, t.ID)
	if err != nil {
		return "", err
	}
	if existing != "" {
		if kept, err := readSection(existing, SubtasksHeading); err == nil && len(kept) > 0 {
			t.Subtasks = kept
		}
		if existing != target {
			// The title changed; move rather than orphan the old note.
			if err := os.Rename(existing, target); err != nil {
				return "", err
			}
		}
	}

	if err := os.WriteFile(target, []byte(renderTaskNote(t)), 0o644); err != nil {
		return "", err
	}
	return target, nil
}

// FindNoteByID looks for a note in dir carrying the given taskii_id.
func FindNoteByID(dir, id string) (string, error) {
	if id == "" {
		return "", nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	want := "taskii_id: " + id
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// Only the frontmatter is inspected, so the id cannot be matched from
		// body text that merely mentions it.
		head := string(b)
		if end := strings.Index(head, "\n---"); end > 0 {
			head = head[:end]
		}
		for _, line := range strings.Split(head, "\n") {
			if strings.TrimSpace(line) == want {
				return path, nil
			}
		}
	}
	return "", nil
}

// readSection returns the non-blank lines under a heading.
func readSection(path, heading string) ([]string, error) {
	lines, _, err := readLines(path)
	if err != nil {
		return nil, err
	}
	start := findHeading(lines, heading)
	if start < 0 {
		return nil, nil
	}
	end := sectionEnd(lines, start)
	var out []string
	for i := start + 1; i < end; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			out = append(out, lines[i])
		}
	}
	return out, nil
}

// ArchiveTaskNote moves a finished task's note into archiveDir.
//
// taskii moves this itself rather than relying on the vault's own note mover:
// every rule there is scoped to the tickets folder, so a note anywhere else is
// never picked up.
func ArchiveTaskNote(notePath, archiveDir string) (string, error) {
	if notePath == "" {
		return "", nil
	}
	if _, err := os.Stat(notePath); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(archiveDir, filepath.Base(notePath))
	if target == notePath {
		return notePath, nil
	}
	// A name already taken in the archive gets a suffix rather than silently
	// replacing whatever is there.
	target = uniquePath(target)
	if err := os.Rename(notePath, target); err != nil {
		return "", err
	}
	return target, nil
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", stem, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return path
}

// WriteDailyNote writes the day's notes board to a dated note.
//
// The board is the source of truth for its own day, so the file is rewritten
// rather than appended to; otherwise a note deleted from the board would
// linger in the vault forever.
func WriteDailyNote(dir string, day time.Time, notes []string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	stamp := day.Format("2006-01-02")
	path := filepath.Join(dir, stamp+".md")

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("type: daily\n")
	fmt.Fprintf(&b, "date: %s\n", stamp)
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n\n", day.Format("Monday, 02 January 2006"))
	if len(notes) == 0 {
		b.WriteString("_No notes for this day._\n")
	}
	for _, n := range notes {
		// A multi-line note stays one bullet, with continuation lines
		// indented so the list structure survives.
		lines := strings.Split(strings.TrimRight(n, "\n"), "\n")
		fmt.Fprintf(&b, "- %s\n", lines[0])
		for _, cont := range lines[1:] {
			fmt.Fprintf(&b, "  %s\n", cont)
		}
	}

	data := []byte(b.String())
	if existing, err := os.ReadFile(path); err == nil && string(existing) == string(data) {
		// Unchanged content is not rewritten: this folder is watched and
		// committed, and a no-op write would show up as a change.
		return path, nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
