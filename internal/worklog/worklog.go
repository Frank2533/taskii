// Package worklog accrues time against Jira issue keys.
//
// Time is local by default and stays that way. The store lives in taskii's own
// data directory, outside any vault, so nothing here is committed or pushed;
// flushing it to a note or on to Jira is an explicit, separate step.
package worklog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"taskii/internal/model"
)

const fileName = "worklog.json"

// Session is one tracked stretch of work.
type Session struct {
	Start   time.Time `json:"start"`
	Seconds int       `json:"seconds"`
}

// Entry is everything tracked against one issue key.
type Entry struct {
	Key      string    `json:"key"`
	Seconds  int       `json:"seconds"`
	Sessions []Session `json:"sessions,omitempty"`

	// FlushedSeconds is how much of Seconds has already been written out, so
	// a second flush reports only what is new rather than double-counting.
	FlushedSeconds int `json:"flushed_seconds,omitempty"`
}

// Pending is time tracked but not yet written anywhere.
func (e Entry) Pending() time.Duration {
	d := e.Seconds - e.FlushedSeconds
	if d < 0 {
		return 0
	}
	return time.Duration(d) * time.Second
}

// Log is the whole store.
type Log struct {
	Entries map[string]*Entry `json:"entries"`
}

func path() string { return filepath.Join(model.DataDir(), fileName) }

func Load() (*Log, error) {
	l := &Log{Entries: map[string]*Entry{}}
	b, err := os.ReadFile(path())
	if err != nil {
		if os.IsNotExist(err) {
			return l, nil
		}
		return l, err
	}
	if len(b) == 0 {
		return l, nil
	}
	if err := json.Unmarshal(b, l); err != nil {
		return &Log{Entries: map[string]*Entry{}}, err
	}
	if l.Entries == nil {
		l.Entries = map[string]*Entry{}
	}
	return l, nil
}

func (l *Log) Save() error {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	dir := model.DataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path(), b, 0o644)
}

// Add records time against a key.
func (l *Log) Add(key string, d time.Duration, at time.Time) {
	if key == "" || d <= 0 {
		return
	}
	if l.Entries == nil {
		l.Entries = map[string]*Entry{}
	}
	e, ok := l.Entries[key]
	if !ok {
		e = &Entry{Key: key}
		l.Entries[key] = e
	}
	secs := int(d.Round(time.Second) / time.Second)
	e.Seconds += secs
	e.Sessions = append(e.Sessions, Session{Start: at, Seconds: secs})
}

// Get returns the entry for a key.
func (l *Log) Get(key string) Entry {
	if e, ok := l.Entries[key]; ok {
		return *e
	}
	return Entry{Key: key}
}

// MarkFlushed records that d has been written out for key.
func (l *Log) MarkFlushed(key string, d time.Duration) {
	e, ok := l.Entries[key]
	if !ok {
		return
	}
	e.FlushedSeconds += int(d.Round(time.Second) / time.Second)
	if e.FlushedSeconds > e.Seconds {
		e.FlushedSeconds = e.Seconds
	}
}

// Keys lists tracked keys, most time first.
func (l *Log) Keys() []string {
	out := make([]string, 0, len(l.Entries))
	for k := range l.Entries {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := l.Entries[out[i]], l.Entries[out[j]]
		if a.Seconds != b.Seconds {
			return a.Seconds > b.Seconds
		}
		return out[i] < out[j]
	})
	return out
}

// Format renders a duration the way Jira's worklog field expects: "2h 15m".
// Anything under a minute rounds up, since Jira rejects a zero-length entry.
func Format(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	total := int(d.Round(time.Minute) / time.Minute)
	if total == 0 {
		total = 1
	}
	h, m := total/60, total%60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dm", m)
	}
}
