// Package vault makes small, line-scoped edits to notes in an Obsidian vault.
//
// Every edit here rewrites as little as possible. Obsidian may have the same
// note open, plugins rewrite parts of it on their own schedule, and a sync
// process is watching for changes — so replacing a whole file to change one
// character invites losing someone else's concurrent edit. Writes are atomic
// (temp file in the same directory, then rename) so a reader never observes a
// half-written note.
package vault

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrStale means the file no longer matches what the caller last read. The
// vault renames and rewrites notes on its own, so an index entry can describe
// a file that has already moved on.
type ErrStale struct{ Detail string }

func (e ErrStale) Error() string { return "note changed since it was read: " + e.Detail }

// readLines loads a note, normalising line endings so indices are stable.
func readLines(path string) ([]string, os.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	return strings.Split(string(b), "\n"), info.Mode().Perm(), nil
}

// writeLines replaces the note atomically.
func writeLines(path string, lines []string, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".taskii-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.WriteString(strings.Join(lines, "\n")); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// AppendUnderHeading inserts text as the last line of the section introduced by
// heading, creating the section at the end of the note if it is absent.
//
// "Last line of the section" means after the section's final non-blank line,
// not at the very end of the file — so a new checkbox joins the existing list
// instead of drifting below the blank line that separates sections.
func AppendUnderHeading(path, heading, text string) error {
	lines, mode, err := readLines(path)
	if err != nil {
		return err
	}

	start := findHeading(lines, heading)
	if start < 0 {
		trimmed := trimTrailingBlank(lines)
		trimmed = append(trimmed, "", "## "+heading, text, "")
		return writeLines(path, trimmed, mode)
	}

	end := sectionEnd(lines, start)
	insert := end
	for insert > start+1 && strings.TrimSpace(lines[insert-1]) == "" {
		insert--
	}

	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:insert]...)
	out = append(out, text)
	out = append(out, lines[insert:]...)
	return writeLines(path, out, mode)
}

// AppendCheckbox adds an unchecked task under heading.
func AppendCheckbox(path, heading, text string) error {
	return AppendUnderHeading(path, heading, "- [ ] "+strings.TrimSpace(text))
}

// SetCheckbox rewrites a single checkbox line's marker.
//
// wantText is the task text the caller believes is on that line; the edit is
// refused if it does not match, because the file may have been rewritten since
// the index was built and toggling the wrong line silently corrupts a list.
func SetCheckbox(path string, line int, wantText string, done bool) error {
	lines, mode, err := readLines(path)
	if err != nil {
		return err
	}
	if line < 0 || line >= len(lines) {
		return ErrStale{Detail: fmt.Sprintf("line %d is outside the note", line)}
	}
	current := lines[line]
	trimmed := strings.TrimSpace(current)
	gotDone, gotText, ok := parseCheckbox(trimmed)
	if !ok {
		return ErrStale{Detail: fmt.Sprintf("line %d is no longer a task line", line)}
	}
	if wantText != "" && gotText != wantText {
		return ErrStale{Detail: fmt.Sprintf("line %d now reads %q", line, gotText)}
	}
	if gotDone == done {
		return nil
	}
	marker := " "
	if done {
		marker = "x"
	}
	// Preserve the original indentation and bullet character.
	idx := strings.Index(current, "[")
	if idx < 0 || idx+2 >= len(current) {
		return ErrStale{Detail: fmt.Sprintf("line %d has no checkbox marker", line)}
	}
	lines[line] = current[:idx+1] + marker + current[idx+2:]
	return writeLines(path, lines, mode)
}

func parseCheckbox(trimmed string) (done bool, text string, ok bool) {
	for _, bullet := range []string{"- ", "* ", "+ "} {
		if !strings.HasPrefix(trimmed, bullet) {
			continue
		}
		rest := trimmed[len(bullet):]
		if len(rest) < 3 || rest[0] != '[' || rest[2] != ']' {
			return false, "", false
		}
		return rest[1] != ' ', strings.TrimSpace(rest[3:]), true
	}
	return false, "", false
}

// SetProperty sets a top-level frontmatter key, adding it if absent.
//
// Only scalar keys are handled; a key whose value spans following lines (a
// block list) is refused rather than mangled.
func SetProperty(path, key, value string) error {
	lines, mode, err := readLines(path)
	if err != nil {
		return err
	}
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ErrStale{Detail: "note has no frontmatter"}
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return ErrStale{Detail: "frontmatter is not closed"}
	}

	prefix := key + ":"
	for i := 1; i < end; i++ {
		if !strings.HasPrefix(lines[i], prefix) {
			continue
		}
		if i+1 < end && strings.HasPrefix(strings.TrimLeft(lines[i+1], " "), "- ") {
			return ErrStale{Detail: fmt.Sprintf("%q holds a list", key)}
		}
		lines[i] = key + ": " + value
		return writeLines(path, lines, mode)
	}

	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:end]...)
	out = append(out, key+": "+value)
	out = append(out, lines[end:]...)
	return writeLines(path, out, mode)
}

// findHeading returns the index of the line introducing heading, or -1.
func findHeading(lines []string, heading string) int {
	want := strings.ToLower(strings.TrimSpace(heading))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.ToLower(strings.TrimSpace(strings.TrimLeft(trimmed, "#"))) == want {
			return i
		}
	}
	return -1
}

// sectionEnd returns the index just past the section that starts at start.
func sectionEnd(lines []string, start int) int {
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "#") {
			return i
		}
	}
	return len(lines)
}

func trimTrailingBlank(lines []string) []string {
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[:end]
}
