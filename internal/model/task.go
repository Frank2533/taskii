package model

import "time"

// Kind distinguishes a plain task from an appointment. Appointments are the
// only entries that carry a Time; a "" Kind in old saved data (before this
// field existed) is treated as KindTask by IsAppointment below, so existing
// task lists keep working unmodified.
type Kind string

const (
	KindTask        Kind = "task"
	KindAppointment Kind = "appointment"
)

type Task struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Done      bool       `json:"done"`
	Important bool       `json:"important,omitempty"`
	Kind      Kind       `json:"kind,omitempty"`
	Date      string     `json:"date"` // YYYY-MM-DD
	Time      string     `json:"time"` // HH:MM; only appointments set this
	CreatedAt time.Time  `json:"created_at"`
	DoneAt    *time.Time `json:"done_at,omitempty"`

	// Due is when the work must be finished by.
	//
	// It is deliberately separate from Date. Date is the day a task is filed
	// under and decides which list it appears in; a deadline several days out
	// would remove the task from Today until that day arrived. A task can be
	// worked on today and due on Friday, and those are different facts.
	Due *time.Time `json:"due,omitempty"`

	// RemindAt is when to nudge the user to START, as distinct from when the
	// work is due.
	RemindAt *time.Time `json:"remind_at,omitempty"`

	// Reminded records that RemindAt has already fired, so restarting the app
	// does not replay every reminder that has passed.
	Reminded bool `json:"reminded,omitempty"`

	// TicketKey links this row to a Jira ticket in the vault. Tickets are
	// added to the dashboard explicitly rather than mirrored wholesale, so
	// the list stays a chosen working set instead of a feed.
	TicketKey string `json:"ticket_key,omitempty"`

	// Collapsed hides a ticket's subtasks in the dashboard.
	Collapsed bool `json:"collapsed,omitempty"`

	// NotePath is this task's note in the vault, when Obsidian sync is on.
	// Cached so the note can be found without rescanning, and re-resolved by
	// taskii_id whenever it turns out to be wrong.
	NotePath string `json:"note_path,omitempty"`
}

// IsTicket reports whether this row stands for a vault ticket.
func (t Task) IsTicket() bool { return t.TicketKey != "" }

// HasDue reports whether a deadline is set.
func (t Task) HasDue() bool { return t.Due != nil && !t.Due.IsZero() }

// Deadline returns the deadline, or the zero time.
func (t Task) Deadline() time.Time {
	if t.Due == nil {
		return time.Time{}
	}
	return *t.Due
}

// PastDue reports whether the deadline has been missed and the work is not
// done.
func (t Task) PastDue(now time.Time) bool {
	return !t.Done && t.HasDue() && now.After(*t.Due)
}

func (t Task) IsAppointment() bool {
	return t.Kind == KindAppointment
}
