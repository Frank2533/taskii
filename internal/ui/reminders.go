package ui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"taskii/internal/model"
	"taskii/internal/notify"
	"taskii/internal/para"
	"taskii/internal/remind"
)

// reminderTickMsg drives the reminder sweep.
type reminderTickMsg time.Time

// reminderFiredMsg reports which tasks were nudged, so the UI can show a
// banner alongside the desktop notification.
type reminderFiredMsg struct {
	titles []string
}

// reminderInterval is how often due reminders are swept for.
//
// Reminders are whole-minute promises, so a minute's granularity is enough and
// keeps the app idle between checks rather than waking every second.
const reminderInterval = 30 * time.Second

func reminderTick() tea.Cmd {
	return tea.Tick(reminderInterval, func(t time.Time) tea.Msg {
		return reminderTickMsg(t)
	})
}

// dueReminders returns the indices of tasks whose start reminder has come due
// and not yet fired.
//
// Firing is recorded on the task rather than held in memory, so restarting the
// app does not replay every reminder that passed while it was closed.
func dueReminders(tasks []model.Task, now time.Time) []int {
	var out []int
	for i, t := range tasks {
		if t.Done || t.Reminded || t.RemindAt == nil {
			continue
		}
		if !now.Before(*t.RemindAt) {
			out = append(out, i)
		}
	}
	return out
}

// desktopSend is the OS notification call, indirected so tests can silence it
// without also switching off persistence, which they need to exercise.
var desktopSend = sendDesktopNotification

// sendDesktopNotification is the real implementation. Every mechanism is optional: a machine
// without a notifier still gets the in-app banner, so a missing binary degrades
// rather than breaking.
func desktopNotify(title, message string) tea.Cmd {
	return func() tea.Msg {
		desktopSend(title, message)
		return nil
	}
}

func sendDesktopNotification(title, message string) {
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`display notification %q with title %q sound name %q`,
			message, title, notificationSound)
		_ = exec.Command("osascript", "-e", script).Run()
	case "linux":
		_ = exec.Command("notify-send", title, message).Run()
		playSoundLinux()
	}
}

// announce delivers a notification everywhere the user has asked for one.
//
// The desktop notification is sent regardless; the phone push is attempted
// only when it is configured, and its failure is reported without disturbing
// the desktop one, which has already been shown.
func (a App) announce(title, message string, tags ...string) tea.Cmd {
	var cmds []tea.Cmd
	// A mock run is a demo or a test: it must not spray the desktop with
	// notifications, for the same reason it does not write to disk.
	if !a.noPersist {
		cmds = append(cmds, desktopNotify(title, message))
	}
	cfg := a.pushConfig()
	if !cfg.Ready() {
		if !a.noPersist {
			detail := "phone notifications are off"
			if a.pushEnabled {
				detail = "no ntfy topic is set"
			}
			_ = notify.Append(model.DataDir(), notify.Entry{
				At: a.now(), Title: title, Message: message,
				Outcome: notify.Desktop, Detail: detail,
			})
		}
		return tea.Batch(cmds...)
	}
	quiet := a.noPersist
	dir := model.DataDir()
	cmds = append(cmds, func() tea.Msg {
		err := notify.Push(context.Background(), cfg, notify.Message{
			Title: title, Body: message, Tags: tags, Priority: 4,
		})
		if quiet {
			if err != nil {
				return pushFailedMsg{err: err}
			}
			return nil
		}
		entry := notify.Entry{At: time.Now(), Title: title, Message: message, Outcome: notify.Sent}
		if err != nil {
			entry.Outcome, entry.Detail = notify.Failed, err.Error()
		}
		_ = notify.Append(dir, entry)
		if err != nil {
			return pushFailedMsg{err: err}
		}
		return nil
	})
	return tea.Batch(cmds...)
}

// pushConfig is the phone notification settings.
func (a App) pushConfig() notify.Config {
	return notify.Config{Enabled: a.pushEnabled, Server: a.ntfyServer, Topic: a.ntfyTopic}
}

// pushFailedMsg reports that a phone notification could not be delivered.
type pushFailedMsg struct{ err error }

// fireEventReminders delivers the 15, 5 and 1 minute warnings for events.
func (a *App) fireEventReminders() tea.Cmd {
	if len(a.events) == 0 {
		return nil
	}
	if a.fired == nil {
		a.fired = remind.NewFired()
	}
	due, late := remind.Pending(a.events, a.now(), a.fired)
	if !a.noPersist {
		_ = a.fired.Save(model.DataDir())
		// Record what was never sent, so a missing notification can be told
		// apart from a broken one.
		for _, d := range late {
			_ = notify.Append(model.DataDir(), notify.Entry{
				At: a.now(), Title: "taskii", Message: d.Message(),
				Outcome: notify.Skipped,
				Detail:  "due while taskii was not running",
			})
		}
	}
	if len(due) == 0 {
		return nil
	}

	var cmds []tea.Cmd
	var lines []string
	for _, d := range due {
		lines = append(lines, d.Message())
		cmds = append(cmds, a.announce("taskii", d.Message(), "calendar"))
	}
	cmds = append(cmds, func() tea.Msg { return reminderFiredMsg{titles: lines} })
	return tea.Batch(cmds...)
}

// subtaskReminders collects every reminder written into a task line in the
// vault, from tickets and local tasks alike.
func (a App) subtaskReminders() []remind.Line {
	if a.idx == nil {
		return nil
	}
	var out []remind.Line
	collect := func(path string, boxes []para.Checkbox) {
		for _, c := range boxes {
			if !c.HasRemind {
				continue
			}
			out = append(out, remind.Line{
				NotePath: path, Text: c.Text, RemindAt: c.RemindAt, Done: c.Done,
			})
		}
	}
	for _, t := range a.idx.Tickets {
		collect(t.Path, t.Checkboxes)
	}
	for _, lt := range a.idx.LocalTasks {
		collect(lt.Path, lt.Checkboxes)
	}
	return out
}

// fireSubtaskReminders delivers reminders set on task lines in the vault.
//
// These need their own pass: the note is the record, so nothing in taskii's
// task store knows about them, and the sweep over tasks never saw them.
func (a *App) fireSubtaskReminders() tea.Cmd {
	lines := a.subtaskReminders()
	if len(lines) == 0 {
		return nil
	}
	if a.fired == nil {
		a.fired = remind.NewFired()
	}
	due, late := remind.PendingLines(lines, a.now(), a.fired)
	if !a.noPersist {
		_ = a.fired.Save(model.DataDir())
		for _, d := range late {
			_ = notify.Append(model.DataDir(), notify.Entry{
				At: a.now(), Title: "taskii", Message: d.Message(),
				Outcome: notify.Skipped,
				Detail:  "due while taskii was not running",
			})
		}
	}
	if len(due) == 0 {
		return nil
	}

	var cmds []tea.Cmd
	var titles []string
	for _, d := range due {
		titles = append(titles, d.Title)
		cmds = append(cmds, a.announce("taskii", d.Message(), "alarm_clock"))
	}
	cmds = append(cmds, func() tea.Msg { return reminderFiredMsg{titles: titles} })
	return tea.Batch(cmds...)
}

// fireReminders marks due reminders as fired, persists that, and returns the
// commands that announce them.
func (a *App) fireReminders() tea.Cmd {
	now := a.now()
	idx := dueReminders(a.tasks, now)
	if len(idx) == 0 {
		return nil
	}

	var titles []string
	for _, i := range idx {
		a.tasks[i].Reminded = true
		titles = append(titles, a.tasks[i].Title)
	}
	a.persist()

	message := "Time to start: " + titles[0]
	if len(titles) > 1 {
		message = fmt.Sprintf("%s (and %d more)", message, len(titles)-1)
	}

	return tea.Batch(
		a.announce("taskii", message, "alarm_clock"),
		func() tea.Msg { return reminderFiredMsg{titles: titles} },
	)
}
