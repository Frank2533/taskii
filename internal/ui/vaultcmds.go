package ui

import (
	"context"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"taskii/internal/export"
	"taskii/internal/ics"
	"taskii/internal/model"
	"taskii/internal/para"
)

// indexMsg carries the result of scanning the vault.
type indexMsg struct {
	idx *para.Index
	err error
}

// actionMsg is the result of one Obsidian CLI call. Those calls cross a
// process boundary into an app that may not even be running, so they are never
// made inline in Update — the UI would freeze for the duration.
type actionMsg struct {
	label string
	out   string
	err   error
}

// icsMsg reports a calendar export.
type icsMsg struct {
	path  string
	wrote bool
	count int
	err   error
}

// icsTickMsg drives the periodic export.
type icsTickMsg time.Time

// loadIndex scans the vault off the UI goroutine.
func loadIndex(vault string, loc *time.Location) tea.Cmd {
	if vault == "" {
		return nil
	}
	return func() tea.Msg {
		idx, err := para.Scan(vault, loc)
		return indexMsg{idx: idx, err: err}
	}
}

// runAction wraps a CLI call so its result arrives as a message.
func runAction(label string, fn func(context.Context) (string, error)) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, err := fn(ctx)
		return actionMsg{label: label, out: out, err: err}
	}
}

// exportICS rewrites the calendar file.
//
// The index is scanned fresh rather than reusing the UI's copy: the export can
// fire from a timer minutes after the last keystroke, and writing a stale
// calendar into a synced folder is worse than spending a few milliseconds.
func exportICS(vault, out string, tasks []model.Task, events []model.Event, loc *time.Location) tea.Cmd {
	if vault == "" || out == "" {
		return nil
	}
	snapshot := make([]model.Task, len(tasks))
	copy(snapshot, tasks)
	eventSnapshot := make([]model.Event, len(events))
	copy(eventSnapshot, events)
	return func() tea.Msg {
		idx, err := para.Scan(vault, loc)
		if err != nil {
			return icsMsg{path: out, err: err}
		}
		cal := export.Calendar(filepath.Base(vault), idx, snapshot, eventSnapshot, loc)
		wrote, err := ics.WriteIfChanged(out, cal.Render())
		return icsMsg{path: out, wrote: wrote, count: len(cal.Events), err: err}
	}
}

// icsTick schedules the next periodic export. A zero or negative interval
// disables it.
func icsTick(every time.Duration) tea.Cmd {
	if every <= 0 {
		return nil
	}
	return tea.Tick(every, func(t time.Time) tea.Msg { return icsTickMsg(t) })
}
