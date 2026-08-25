package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"taskii/internal/model"
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

// notify sends a desktop notification. Every mechanism is optional: a machine
// without a notifier still gets the in-app banner, so a missing binary degrades
// rather than breaking.
func notify(title, message string) tea.Cmd {
	return func() tea.Msg {
		switch runtime.GOOS {
		case "darwin":
			script := fmt.Sprintf(`display notification %q with title %q sound name %q`,
				message, title, notificationSound)
			_ = exec.Command("osascript", "-e", script).Run()
		case "linux":
			_ = exec.Command("notify-send", title, message).Run()
			playSoundLinux()
		}
		return nil
	}
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
		notify("taskii", message),
		func() tea.Msg { return reminderFiredMsg{titles: titles} },
	)
}
