// Package para models an Obsidian vault organised with the PARA method
// (Projects, Areas, Resources, Archive) whose Tickets folder mirrors a Jira
// project.
//
// The vault is read straight off disk rather than through the Obsidian CLI.
// Reads happen on every keystroke, and the CLI costs a process spawn plus IPC
// and needs the GUI running — parsing markdown ourselves keeps the UI
// responsive and keeps the whole read path working while Obsidian is closed.
package para

import (
	"strings"
	"time"
)

// Closed statuses are the ones the vault's Auto Note Mover treats as finished.
// A ticket only leaves Tickets/ when it is closed AND has an Area set, so a
// closed ticket with no Area is stranded — see Index.Unfiled.
var closedStatuses = map[string]bool{
	"done":     true,
	"declined": true,
}

// Checkbox is one "- [ ]" line in a note body, remembered with its line number
// so a toggle can rewrite exactly that line instead of the whole file.
type Checkbox struct {
	Line int // 0-based index into the file's lines
	Done bool
	// Text is what the user wrote, with any scheduling tokens removed, so a
	// task never has to be edited around its own metadata.
	Text    string
	Heading string // the "## ..." section it sits under, "" if none

	// Due and RemindAt are read from the line's own tokens, so a deadline set
	// in taskii is a deadline in the vault rather than a fact only taskii
	// knows.
	Due    time.Time
	HasDue bool

	RemindAt  time.Time
	HasRemind bool

	// Notes is the indented text block written under this task line, which is
	// where a note about one subtask belongs: attached to it, rather than in a
	// section shared by the whole note.
	Notes []string
}

// Ticket is one note in Tickets/ or Archive/<Area>/, mirroring a Jira issue.
type Ticket struct {
	Key       string
	Summary   string
	Status    string
	Area      string
	IssueType string
	Priority  string
	EpicLink  string
	Assignee  string
	Sprint    string
	Link      string

	Due    time.Time
	HasDue bool

	Updated time.Time

	Path       string // absolute
	Archived   bool   // lives under Archive/
	Checkboxes []Checkbox

	// Notes is the ticket's freeform work log. jira-sync rewrites its own
	// sections wholesale on every fetch, so this is the one place a note can
	// be kept on a ticket without being lost at the next sync.
	Notes []string
}

// Closed reports whether the ticket's status is one the archive automation
// acts on.
func (t Ticket) Closed() bool {
	return closedStatuses[strings.ToLower(strings.TrimSpace(t.Status))]
}

// Stranded is a closed ticket still sitting in Tickets/ with no Area set. The
// vault's mover requires both conditions, so these are invisible everywhere
// else: Jira considers them finished and the vault never files them.
func (t Ticket) Stranded() bool {
	return t.Closed() && !t.Archived && strings.TrimSpace(t.Area) == ""
}

// Title is what to show in a list.
func (t Ticket) Title() string {
	switch {
	case t.Key != "" && t.Summary != "":
		return t.Key + " " + t.Summary
	case t.Summary != "":
		return t.Summary
	default:
		return t.Key
	}
}

// Project is a note in Projects/ — a cohesive effort with an end state, which
// may span several tickets via JiraEpic.
type Project struct {
	Title    string
	Area     string
	Status   string
	JiraEpic string
	Path     string

	Target    time.Time
	HasTarget bool

	Checkboxes []Checkbox
}

// LocalTask is a task taskii created in the vault, as opposed to a ticket
// mirrored from Jira. It is recognised by its taskii_id, wherever the note
// lives, so moving or renaming it does not break the link.
type LocalTask struct {
	ID       string
	Title    string
	Done     bool
	Path     string
	Archived bool

	Due    time.Time
	HasDue bool

	Checkboxes []Checkbox

	// Notes is the body of the task's own note.
	Notes []string
}

// Area is one ongoing responsibility, one folder under Areas/.
type Area struct {
	Name string
	Path string // the Info.md note, "" if the folder has none
}
