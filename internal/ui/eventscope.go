package ui

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"taskii/internal/model"
)

// scopeAction is what the pending scope prompt will carry out.
type scopeAction int

const (
	scopeEdit scopeAction = iota
	scopeDelete
)

// pendingScope is a change to a recurring event, waiting on the user to say
// how much of the series it applies to.
//
// Changing a repeat is ambiguous in a way a one-off is not — "move the standup
// to ten" could mean tomorrow's, every one from now on, or the whole series —
// and guessing silently rewrites a series the user cannot easily restore.
type pendingScope struct {
	open       bool
	action     scopeAction
	eventID    string
	occurrence time.Time
	edited     model.Event // the parsed replacement, for an edit

	// dateExplicit records whether the edit named a date. When it did not,
	// the parser defaulted to today — which for "this occurrence" or "this
	// and future" is the wrong anchor and would drag the change back over
	// occurrences it was never meant to touch.
	dateExplicit bool
}

// beginScopePrompt asks how far a change to a repeating event reaches.
func (a App) beginScopePrompt(action scopeAction, ev model.Event, occurrence time.Time, edited model.Event, dateExplicit bool) (tea.Model, tea.Cmd) {
	a.scope = pendingScope{
		open: true, action: action, eventID: ev.ID,
		occurrence: occurrence, edited: edited, dateExplicit: dateExplicit,
	}
	return a, nil
}

// anchorTo moves an event onto a given day, keeping its time of day and
// duration. A multi-day event keeps the same span.
func anchorTo(e model.Event, day time.Time) model.Event {
	dur := e.Duration()
	y, m, d := day.Date()
	e.Start = time.Date(y, m, d, e.Start.Hour(), e.Start.Minute(), 0, 0, day.Location())
	e.End = e.Start.Add(dur)
	return e
}

func (a App) renderScopePrompt() string {
	muted := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	text := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)
	key := lipgloss.NewStyle().Foreground(colorAccent).Background(colorPaneBg).Bold(true)

	verb := "Edit"
	if a.scope.action == scopeDelete {
		verb = "Delete"
	}
	title := ""
	if ev, ok := a.eventByID(a.scope.eventID); ok {
		title = ev.Title
	}

	row := func(k, label string) string {
		return key.Render("  "+k+"  ") + text.Render(label)
	}
	lines := []string{
		muted.Render("  " + title + " repeats. " + verb + " which?"),
		"",
		row("t", "this occurrence only ("+a.scope.occurrence.Format("Mon 02 Jan")+")"),
		row("f", "this and all future occurrences"),
		row("a", "the whole series, past included"),
		"",
		muted.Render("  esc  cancel"),
	}
	return renderPane(verb+" recurring event", strings.Join(lines, "\n"), true, 56, len(lines)+2)
}

// eventByID finds a stored event.
func (a App) eventByID(id string) (model.Event, bool) {
	for _, e := range a.events {
		if e.ID == id {
			return e, true
		}
	}
	return model.Event{}, false
}

// updateScopePrompt applies the chosen scope.
func (a App) updateScopePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		a.scope = pendingScope{}
		return a, nil
	case "t":
		return a.applyScope(scopeThis)
	case "f":
		return a.applyScope(scopeFuture)
	case "a":
		return a.applyScope(scopeAll)
	}
	return a, nil
}

// scopeChoice is how much of a series a change reaches.
type scopeChoice int

const (
	scopeThis scopeChoice = iota
	scopeFuture
	scopeAll
)

// applyScope carries out the pending change.
func (a App) applyScope(choice scopeChoice) (tea.Model, tea.Cmd) {
	pending := a.scope
	a.scope = pendingScope{}

	idx := -1
	for i := range a.events {
		if a.events[i].ID == pending.eventID {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.setErr("that event no longer exists")
		return a, nil
	}
	original := a.events[idx]

	switch choice {
	case scopeAll:
		if pending.action == scopeDelete {
			a.events = append(a.events[:idx:idx], a.events[idx+1:]...)
			a.setStatus("deleted " + original.Title)
		} else {
			edited := pending.edited
			// The id is kept so the series stays the same entry for anyone
			// subscribed, and so reminders already sent are still recognised.
			edited.ID = original.ID
			edited.Except = original.Except
			a.events[idx] = edited
			a.setStatus("updated the whole series")
		}

	case scopeThis:
		// The occurrence is removed from the series. An edit then re-adds it
		// as a one-off, so the series itself is never rewritten.
		original.Except = append(original.Except, pending.occurrence)
		a.events[idx] = original
		if pending.action == scopeDelete {
			a.setStatus("removed " + pending.occurrence.Format("Mon 02 Jan"))
		} else {
			one := pending.edited
			if !pending.dateExplicit {
				one = anchorTo(one, pending.occurrence)
			}
			one.ID = newEventID(a.now(), 1)
			one.Repeat = model.RepeatNone
			one.Days = nil
			one.Interval = 0
			one.Except = nil
			a.events = append(a.events, one)
			a.setStatus("updated just " + pending.occurrence.Format("Mon 02 Jan"))
		}

	case scopeFuture:
		// The original stops just before this occurrence and, for an edit, a
		// new series takes over from it. Splitting rather than editing in
		// place keeps every past occurrence exactly as it was.
		until := pending.occurrence.Add(-time.Second)
		if !until.After(original.Start) {
			// Nothing precedes this occurrence, so there is no split to make.
			if pending.action == scopeDelete {
				a.events = append(a.events[:idx:idx], a.events[idx+1:]...)
				a.setStatus("deleted " + original.Title)
				break
			}
			edited := pending.edited
			edited.ID = original.ID
			a.events[idx] = edited
			a.setStatus("updated the whole series")
			break
		}
		original.Until = &until
		a.events[idx] = original
		if pending.action == scopeDelete {
			a.setStatus("ended the series before " + pending.occurrence.Format("Mon 02 Jan"))
		} else {
			rest := pending.edited
			if !pending.dateExplicit {
				rest = anchorTo(rest, pending.occurrence)
			}
			rest.ID = newEventID(a.now(), 1)
			rest.Except = nil
			a.events = append(a.events, rest)
			a.setStatus("updated this and future occurrences")
		}
	}

	if !a.noPersist {
		_ = model.SaveEvents(a.events)
	}
	return a, exportICS(a.vaultPath, a.icsOut, a.tasks, a.events, a.loc)
}

// newEventID mints an id. The offset keeps a split from colliding with the
// event it was split from when both are created in the same nanosecond.
func newEventID(now time.Time, offset int64) string {
	return strconv.FormatInt(now.UnixNano()+offset, 36)
}
