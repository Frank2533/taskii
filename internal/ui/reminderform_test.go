package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// taskWithPicker opens the deadline picker on a plain task.
func taskWithPicker(t *testing.T) App {
	t.Helper()
	a, _ := dashApp(t)
	// Pin the clock so an offset can be asserted exactly. It stays on today's
	// date, which the vault fixture's due dates are relative to.
	fixed := time.Now()
	a.now = func() time.Time { return fixed }
	a = press(t, a, "a")
	a = typeInto(a, "buy oat milk")
	a = press(t, a, "enter")
	a.todaySelected = 0
	a = press(t, a, "D")
	if !a.picker.open {
		t.Fatal("picker did not open")
	}
	return a
}

func TestPickerOffersCustomReminder(t *testing.T) {
	a := taskWithPicker(t)
	if !strings.Contains(a.View(), "custom") {
		t.Errorf("the picker does not offer a custom reminder:\n%s", a.View())
	}
}

func TestCustomFormOpensWithDaysAtZero(t *testing.T) {
	a := press(t, taskWithPicker(t), "c")
	if !a.form.open {
		t.Fatal("the form did not open")
	}
	if a.form.values[fieldDays] != "0" {
		t.Errorf("days = %q, want it defaulted to 0", a.form.values[fieldDays])
	}
	if a.form.field != fieldDays {
		t.Errorf("cursor starts on %v, want days", a.form.field)
	}
	out := a.View()
	for _, want := range []string{"Custom reminder", "days", "hours", "minutes"} {
		if !strings.Contains(out, want) {
			t.Errorf("the form is missing %q:\n%s", want, out)
		}
	}
}

func TestCustomFormSetsAReminder(t *testing.T) {
	a := press(t, taskWithPicker(t), "c")
	// 2 days, 3 hours, 30 minutes.
	a = press(t, a, "2", "tab", "3", "tab", "3", "0")
	want := 2*24*time.Hour + 3*time.Hour + 30*time.Minute
	if got := a.form.duration(); got != want {
		t.Fatalf("duration = %v, want %v", got, want)
	}
	if !strings.Contains(a.View(), "2 days 3 hours 30 minutes") {
		t.Errorf("the offset is not described back:\n%s", a.View())
	}

	before := a.now()
	a = press(t, a, "enter")
	if a.form.open || a.picker.open {
		t.Error("the form or picker stayed open after setting")
	}
	if len(a.tasks) != 1 || a.tasks[0].RemindAt == nil {
		t.Fatalf("no reminder was set: %+v", a.tasks)
	}
	// The offset runs from now, which is what setting it here means.
	if got := a.tasks[0].RemindAt.Sub(before); got != want {
		t.Errorf("reminder is %v away, want %v", got, want)
	}
}

// The days box starts at "0"; typing must replace it, not append.
func TestLeadingZeroIsReplaced(t *testing.T) {
	a := press(t, taskWithPicker(t), "c")
	a = press(t, a, "5")
	if a.form.values[fieldDays] != "5" {
		t.Errorf("days = %q, want 5", a.form.values[fieldDays])
	}
}

func TestCustomFormRejectsAnEmptyOffset(t *testing.T) {
	a := press(t, taskWithPicker(t), "c")
	a = press(t, a, "enter")
	if !a.form.open {
		t.Error("the form closed on an offset of zero")
	}
	if a.form.err == "" {
		t.Error("no explanation was given")
	}
	if a.tasks[0].RemindAt != nil {
		t.Error("a zero offset set a reminder")
	}
}

func TestCustomFormBackspaceAndCancel(t *testing.T) {
	a := press(t, taskWithPicker(t), "c")
	a = press(t, a, "tab", "9")
	if a.form.values[fieldHours] != "9" {
		t.Fatalf("hours = %q", a.form.values[fieldHours])
	}
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	a = m.(App)
	if a.form.values[fieldHours] != "" {
		t.Errorf("backspace left %q", a.form.values[fieldHours])
	}
	a = press(t, a, "esc")
	if a.form.open {
		t.Error("esc did not close the form")
	}
	if a.tasks[0].RemindAt != nil {
		t.Error("cancelling still set a reminder")
	}
}

// A subtask's reminder belongs in its note, like its deadline.
func TestCustomReminderOnASubtaskWritesToTheNote(t *testing.T) {
	a, root := dashApp(t)
	a = press(t, a, "a")
	a = typeInto(a, "normalize")
	a = press(t, a, "down", "enter")

	a.todaySelected = 2 // a subtask
	a = press(t, a, "D", "c")
	if !a.form.open {
		t.Fatal("the form did not open on a subtask")
	}
	a = press(t, a, "tab", "2") // 2 hours
	next, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a = next.(App)
	if a.err != "" {
		t.Fatalf("setting reported: %s", a.err)
	}
	if cmd == nil {
		t.Error("writing to a note should reindex")
	}

	body := readFile(t, root, "Tickets", "AAA-1 In Progress live.md")
	if !strings.Contains(body, "(@") {
		t.Errorf("the reminder was not written into the task line:\n%s", body)
	}
}

// readFile is a small helper for asserting on a vault note's contents.
func readFile(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
