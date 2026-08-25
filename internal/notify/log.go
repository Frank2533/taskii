package notify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const logFile = "notifications.log"

// maxLogEntries bounds the file. It is rewritten rather than appended to
// forever, so a long-running install cannot grow it without limit.
const maxLogEntries = 500

// Outcome is what happened to one delivery attempt.
type Outcome string

const (
	// Sent means the server accepted it.
	Sent Outcome = "sent"
	// Failed means it was attempted and rejected or unreachable.
	Failed Outcome = "failed"
	// Skipped means it was never attempted, and Detail says why. A reminder
	// that was due while the app was closed is recorded this way — without it
	// a missing notification is indistinguishable from a broken one.
	Skipped Outcome = "skipped"
	// Desktop records a local notification, which has no server to accept it.
	Desktop Outcome = "desktop"
)

// Entry is one line of the log.
type Entry struct {
	At      time.Time `json:"at"`
	Title   string    `json:"title"`
	Message string    `json:"message"`
	Outcome Outcome   `json:"outcome"`
	Detail  string    `json:"detail,omitempty"`
}

// Line renders an entry for reading.
func (e Entry) Line() string {
	out := e.At.Format("2006-01-02 15:04:05") + "  " + string(e.Outcome)
	for len(out) < 33 {
		out += " "
	}
	out += e.Message
	if e.Detail != "" {
		out += "  (" + e.Detail + ")"
	}
	return out
}

func logPath(dir string) string { return filepath.Join(dir, logFile) }

// Append records a delivery attempt.
//
// The topic is never written here. The log is meant to be read, pasted and
// shared when something is not arriving, and the topic is the one thing in
// this feature that has to stay private.
func Append(dir string, e Entry) error {
	entries, _ := ReadLog(dir, 0)
	entries = append(entries, e)
	if len(entries) > maxLogEntries {
		entries = entries[len(entries)-maxLogEntries:]
	}

	var b strings.Builder
	enc := json.NewEncoder(&b)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(logPath(dir), []byte(b.String()), 0o644)
}

// ReadLog returns the most recent entries, oldest first. limit 0 means all.
func ReadLog(dir string, limit int) ([]Entry, error) {
	b, err := os.ReadFile(logPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			// A corrupt line should not hide the rest of the history.
			continue
		}
		out = append(out, e)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}
