package ui

import (
	"time"

	"taskii/internal/deadline"
	"taskii/internal/model"
)

// decorateDeadlines appends a compact deadline marker to each row's title.
//
// The marker rides in the title rather than becoming its own segment because
// renderTaskLine measures and pads every segment up front against the pane
// width; adding a segment means touching that arithmetic, and a mistake there
// shows up as a background hole or a row that resizes as you type. The title
// is already truncated safely, so this cannot overflow.
//
// It operates on copies. Nothing here may reach the persisted task list — a
// decorated title written back to disk would accumulate markers on every save.
func decorateDeadlines(tasks []model.Task, now time.Time) []model.Task {
	out := make([]model.Task, len(tasks))
	copy(out, tasks)
	for i := range out {
		if !out[i].HasDue() {
			continue
		}
		out[i].Title += "  " + deadlineMarker(out[i], now)
	}
	return out
}

// deadlineMarker is the short form shown after a title. A missed deadline gets
// a different glyph, since the row's own colour already carries the overdue
// state only in the Overdue pane — a task due today can be late while still
// sitting in Today.
func deadlineMarker(t model.Task, now time.Time) string {
	short := deadline.Short(t.Deadline(), now)
	if t.PastDue(now) {
		return "⚑ " + short
	}
	return "⌛ " + short
}

// deadlinePhrase is the long form, for the detail line and the timeline.
func deadlinePhrase(t model.Task, now time.Time) string {
	if !t.HasDue() {
		return ""
	}
	return deadline.Describe(t.Deadline(), now)
}
