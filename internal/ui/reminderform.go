package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// reminderField is which box the cursor is in.
type reminderField int

const (
	fieldDays reminderField = iota
	fieldHours
	fieldMinutes

	reminderFieldCount = 3
)

func (f reminderField) label() string {
	switch f {
	case fieldDays:
		return "days"
	case fieldHours:
		return "hours"
	default:
		return "minutes"
	}
}

// reminderForm collects a custom offset as days, hours and minutes.
//
// Three separate boxes rather than one free-text duration: the offsets people
// actually want are mixed units ("2 days and 3 hours"), and a single field
// would have to teach a syntax for that before it could accept one.
type reminderForm struct {
	open   bool
	field  reminderField
	values [reminderFieldCount]string
	err    string
}

// maxDigits bounds each box. Four is enough for any offset worth setting and
// stops a stray key from producing an absurd one.
const maxDigits = 4

// duration is the offset the form describes.
func (f reminderForm) duration() time.Duration {
	num := func(i reminderField) int {
		n, err := strconv.Atoi(f.values[i])
		if err != nil {
			return 0
		}
		return n
	}
	return time.Duration(num(fieldDays))*24*time.Hour +
		time.Duration(num(fieldHours))*time.Hour +
		time.Duration(num(fieldMinutes))*time.Minute
}

// describe renders the offset in words, so what is about to be set is legible
// before it is committed.
func (f reminderForm) describe() string {
	d := f.duration()
	if d <= 0 {
		return ""
	}
	days := int(d / (24 * time.Hour))
	hours := int(d % (24 * time.Hour) / time.Hour)
	mins := int(d % time.Hour / time.Minute)

	var parts []string
	add := func(n int, unit string) {
		if n == 0 {
			return
		}
		if n == 1 {
			parts = append(parts, fmt.Sprintf("%d %s", n, unit))
			return
		}
		parts = append(parts, fmt.Sprintf("%d %ss", n, unit))
	}
	add(days, "day")
	add(hours, "hour")
	add(mins, "minute")
	return strings.Join(parts, " ")
}

// beginReminderForm opens the custom-offset form.
func (a App) beginReminderForm() (tea.Model, tea.Cmd) {
	// Days starts at zero because the common case is a reminder later today;
	// the other boxes start empty so the first digit typed replaces nothing.
	a.form = reminderForm{open: true, values: [reminderFieldCount]string{"0", "", ""}}
	return a, nil
}

// updateReminderForm drives the three boxes.
func (a App) updateReminderForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		a.form = reminderForm{}
		return a, nil

	case "tab", "down", "right":
		a.form.field = (a.form.field + 1) % reminderFieldCount
		return a, nil

	case "shift+tab", "up", "left":
		a.form.field = (a.form.field + reminderFieldCount - 1) % reminderFieldCount
		return a, nil

	case "backspace":
		v := a.form.values[a.form.field]
		if v != "" {
			a.form.values[a.form.field] = v[:len(v)-1]
		}
		a.form.err = ""
		return a, nil

	case "enter":
		d := a.form.duration()
		if d <= 0 {
			a.form.err = "set at least one of days, hours or minutes"
			return a, nil
		}
		phrase := a.form.describe()
		a.form = reminderForm{}
		a.picker.open = false
		if a.applyDeadline(nil, &d) {
			a.setStatus("reminder in " + phrase)
		}
		return a, a.reindexIfNeeded()
	}

	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		v := a.form.values[a.form.field]
		// A leading zero is replaced rather than appended to, so the days box
		// starting at "0" does not turn into "05".
		if v == "0" {
			v = ""
		}
		if len(v) < maxDigits {
			a.form.values[a.form.field] = v + key
		}
		a.form.err = ""
	}
	return a, nil
}

func (a App) renderReminderForm() string {
	muted := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	text := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)
	active := lipgloss.NewStyle().Foreground(colorAccent).Background(colorPanel).Bold(true)

	var boxes []string
	for i := reminderField(0); i < reminderFieldCount; i++ {
		v := a.form.values[i]
		if v == "" {
			v = "0"
		}
		style := text
		if i == a.form.field {
			style = active
		}
		boxes = append(boxes,
			style.Render(fmt.Sprintf(" %4s ", v))+muted.Render(" "+i.label()))
	}

	lines := []string{
		muted.Render("  Remind me in"),
		"  " + strings.Join(boxes, "  "),
		"",
	}
	if phrase := a.form.describe(); phrase != "" {
		when := a.now().Add(a.form.duration())
		lines = append(lines, text.Render(fmt.Sprintf("  in %s — %s", phrase, when.Format("Mon 02 Jan 15:04"))))
	} else {
		lines = append(lines, muted.Render("  type digits to set an offset"))
	}
	if a.form.err != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorDanger).Background(colorPaneBg).Render("  "+a.form.err))
	}
	lines = append(lines, "", muted.Render("  tab  next field    enter  set    esc  cancel"))

	return renderPane("Custom reminder", strings.Join(lines, "\n"), true, 56, len(lines)+2)
}
