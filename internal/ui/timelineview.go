package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"taskii/internal/deadline"
)

// timelineKind orders what shares an hour, most urgent first.
type timelineKind int

const (
	tlAppointment timelineKind = iota
	tlReminder
	tlDeadline
)

// timelineItem is one thing pinned to a time today.
type timelineItem struct {
	at   time.Time
	kind timelineKind
	text string
	late bool
}

func (k timelineKind) glyph() string {
	switch k {
	case tlAppointment:
		return "▸"
	case tlReminder:
		return "⏰"
	default:
		return "⚑"
	}
}

func (k timelineKind) label() string {
	switch k {
	case tlAppointment:
		return ""
	case tlReminder:
		return "start: "
	default:
		return "due: "
	}
}

// timelineItems collects everything happening today that has a clock time,
// from local tasks and from the vault.
//
// Items without a time are excluded here and counted separately: placing an
// untimed task at an arbitrary hour would assert a schedule the user never
// gave.
func (a App) timelineItems() (items []timelineItem, anytime int) {
	now := a.now()
	today := startOfDay(now)
	tomorrow := today.AddDate(0, 0, 1)

	inToday := func(t time.Time) bool {
		return !t.Before(today) && t.Before(tomorrow)
	}

	for _, t := range a.tasks {
		if t.Done {
			continue
		}
		timed := false

		if t.IsAppointment() && t.Time != "" && t.Date == now.Format(dateFormat) {
			if at, err := time.ParseInLocation("2006-01-02 15:04", t.Date+" "+t.Time, now.Location()); err == nil {
				items = append(items, timelineItem{at: at, kind: tlAppointment, text: t.Title, late: at.Before(now)})
				timed = true
			}
		}
		if t.RemindAt != nil && inToday(*t.RemindAt) && !t.Reminded {
			items = append(items, timelineItem{at: *t.RemindAt, kind: tlReminder, text: t.Title, late: t.RemindAt.Before(now)})
			timed = true
		}
		if t.HasDue() && inToday(t.Deadline()) {
			items = append(items, timelineItem{at: t.Deadline(), kind: tlDeadline, text: t.Title, late: t.PastDue(now)})
			timed = true
		}
		if !timed && t.Date == now.Format(dateFormat) {
			anytime++
		}
	}

	// Vault tickets due today are deadlines too; they carry a date but no
	// clock time, so they land at the end of the day.
	if a.idx != nil {
		for _, tk := range a.idx.Tickets {
			if !tk.HasDue || tk.Closed() {
				continue
			}
			due := deadline.EndOfDay(tk.Due.In(now.Location()))
			if !inToday(due) {
				continue
			}
			items = append(items, timelineItem{at: due, kind: tlDeadline, text: tk.Title(), late: now.After(due)})
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].at.Equal(items[j].at) {
			return items[i].at.Before(items[j].at)
		}
		return items[i].kind < items[j].kind
	})
	return items, anytime
}

// timelineBounds is the hour range to draw: the working span, widened to cover
// anything scheduled outside it so nothing is silently off the chart.
func timelineBounds(items []timelineItem, now time.Time) (first, last int) {
	first, last = 9, 18
	if h := now.Hour(); h < first {
		first = h
	} else if h > last {
		last = h
	}
	for _, it := range items {
		if h := it.at.Hour(); h < first {
			first = h
		} else if h > last {
			last = h
		}
	}
	return first, last
}

// renderTimelinePane draws the timeline into a pane of the given size.
//
// It re-derives everything from the current task list on each frame, so the
// dashboard's timeline is correct the moment a deadline or reminder is edited
// — there is no cached copy to invalidate.
func (a App) renderTimelinePane(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	now := a.now()

	items, anytime := a.timelineItems()

	muted := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	text := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)
	late := lipgloss.NewStyle().Foreground(colorDanger).Background(colorPaneBg)
	accent := lipgloss.NewStyle().Foreground(colorAccent).Background(colorPaneBg).Bold(true)

	var rows []string
	add := func(s string) {
		if len(rows) < height-2 {
			rows = append(rows, s)
		}
	}

	if a.reminderBanner != "" && now.Sub(a.reminderBannerAt) < 10*time.Minute {
		add(accent.Render(fitToWidth("⏰ "+a.reminderBanner, width-4)))
		add("")
	}

	byHour := map[int][]timelineItem{}
	for _, it := range items {
		byHour[it.at.Hour()] = append(byHour[it.at.Hour()], it)
	}

	first, last := timelineBounds(items, now)
	nowDrawn := false
	for h := first; h <= last; h++ {
		hourItems := byHour[h]
		// Empty hours are kept so the day reads as a continuous scale rather
		// than a list that happens to have times on it, but they are dropped
		// when the terminal is too short to show the whole span.
		if len(hourItems) == 0 && len(rows) > height-8 {
			continue
		}
		add(muted.Render(fmt.Sprintf("  %02d:00 ┃", h)))

		if h == now.Hour() && !nowDrawn {
			nowDrawn = true
			bar := strings.Repeat("━", maxInt(0, minInt(width-18, 30)))
			rows[len(rows)-1] = muted.Render(fmt.Sprintf("  %02d:00 ┃", h)) +
				accent.Render(fmt.Sprintf("━━ now %s ", now.Format("15:04"))+bar)
		}

		for _, it := range hourItems {
			style := text
			if it.late {
				style = late
			}
			line := fmt.Sprintf("        ┃ %s %s%s  %s",
				it.kind.glyph(), it.kind.label(), it.text, it.at.Format("15:04"))
			add(style.Render(fitToWidth(line, width-4)))
		}
	}

	if len(items) == 0 {
		add(muted.Render("Nothing is scheduled for today."))
	}
	if anytime > 0 {
		add("")
		add(muted.Render(fmt.Sprintf("  anytime today: %d task(s) with no time set", anytime)))
	}

	title := fmt.Sprintf("Today — %s", now.Format("Mon 02 Jan"))
	return renderPane(title, strings.Join(rows, "\n"), false, width, height)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
