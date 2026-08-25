package vault

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Task metadata is written into the task line itself, in the conventions
// Obsidian plugins already read, so a deadline set in taskii is a deadline in
// the vault rather than a fact only taskii knows.
//
//   - "📅 2026-08-27" is the Tasks plugin's due date.
//   - "(@2026-08-27 09:00)" is the Reminder plugin's reminder, which carries a
//     time of day where the due date cannot.
const (
	DueEmoji = "📅"
)

var (
	dueRe    = regexp.MustCompile(`\x{1F4C5}\s*(\d{4}-\d{2}-\d{2})`)
	remindRe = regexp.MustCompile(`\(@(\d{4}-\d{2}-\d{2})(?:[ T](\d{2}:\d{2}))?\)`)
)

// TaskMeta is the scheduling written into a task line.
type TaskMeta struct {
	Due    time.Time
	HasDue bool

	RemindAt  time.Time
	HasRemind bool
}

// ParseTaskLine splits a task's text from its scheduling tokens.
//
// The returned text is what the user actually wrote, with the tokens removed,
// so editing a task never has to step around its own metadata.
func ParseTaskLine(text string, loc *time.Location) (string, TaskMeta) {
	if loc == nil {
		loc = time.Local
	}
	var meta TaskMeta

	if m := dueRe.FindStringSubmatch(text); m != nil {
		if d, err := time.ParseInLocation("2006-01-02", m[1], loc); err == nil {
			// A due date is a whole day, so it falls due at the end of it.
			meta.Due = time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 0, loc)
			meta.HasDue = true
		}
		text = dueRe.ReplaceAllString(text, "")
	}
	if m := remindRe.FindStringSubmatch(text); m != nil {
		layout, value := "2006-01-02", m[1]
		if m[2] != "" {
			layout, value = "2006-01-02 15:04", m[1]+" "+m[2]
		}
		if d, err := time.ParseInLocation(layout, value, loc); err == nil {
			meta.RemindAt = d
			meta.HasRemind = true
		}
		text = remindRe.ReplaceAllString(text, "")
	}
	return strings.TrimSpace(strings.Join(strings.Fields(text), " ")), meta
}

// FormatTaskLine renders a task's text with its scheduling tokens appended.
func FormatTaskLine(text string, meta TaskMeta) string {
	out := strings.TrimSpace(text)
	if meta.HasDue {
		out += " " + DueEmoji + " " + meta.Due.Format("2006-01-02")
	}
	if meta.HasRemind {
		out += fmt.Sprintf(" (@%s)", meta.RemindAt.Format("2006-01-02 15:04"))
	}
	return out
}

// SetTaskSchedule rewrites a task line's scheduling, leaving its text and
// checked state alone.
func SetTaskSchedule(path string, line int, wantText string, meta TaskMeta, loc *time.Location) error {
	lines, mode, err := readLines(path)
	if err != nil {
		return err
	}
	if line < 0 || line >= len(lines) {
		return ErrStale{Detail: fmt.Sprintf("line %d is outside the note", line)}
	}
	current := lines[line]
	_, gotText, ok := parseCheckbox(strings.TrimSpace(current))
	if !ok {
		return ErrStale{Detail: fmt.Sprintf("line %d is no longer a task line", line)}
	}
	bare, _ := ParseTaskLine(gotText, loc)
	if wantText != "" && bare != wantText && gotText != wantText {
		return ErrStale{Detail: fmt.Sprintf("line %d now reads %q", line, bare)}
	}

	close := strings.Index(current, "]")
	if close < 0 {
		return ErrStale{Detail: fmt.Sprintf("line %d has no checkbox marker", line)}
	}
	lines[line] = current[:close+1] + " " + FormatTaskLine(bare, meta)
	return writeLines(path, lines, mode)
}
