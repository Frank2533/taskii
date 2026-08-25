package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// weekdayNames label the columns, Monday first.
var weekdayNames = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// renderCalendarGrid draws the calendar at the current scale.
func (a App) renderCalendarGrid() string {
	height := a.height - a.chromeLines()
	if a.mode == modeAddEvent || a.mode == modeEditEvent {
		height-- // the input takes a line below the grid
	}
	if height < 5 {
		height = 5
	}
	from, to := a.calRange()
	entries := a.calendarEntries(from, to)

	var body string
	switch a.calScale {
	case calWeek:
		body = a.renderWeek(from, byDay(entries), a.width, height-2)
	case calMonth:
		body = a.renderMonth(from, to, byDay(entries), a.width, height-2)
	default:
		body = a.renderYear(from, byDay(entries), a.width, height-2)
	}

	title := fmt.Sprintf("%s — %s  (%d)", a.calScale, a.calTitle(from), len(entries))
	pane := renderPane(title, body, true, a.width, height)
	if a.mode != modeAddEvent && a.mode != modeEditEvent {
		return pane
	}
	// The input sits below the grid rather than inside a day cell: an event
	// is not being typed "into" a day until its date token is read.
	a.input.TextStyle = lipgloss.NewStyle().Foreground(colorText)
	a.input.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent)
	a.input.Cursor.Style = lipgloss.NewStyle().Foreground(colorText)
	a.input.Width = 0
	return lipgloss.JoinVertical(lipgloss.Left, pane, inputPromptStyle.Render("+ ")+a.input.View())
}

func (a App) calTitle(from time.Time) string {
	switch a.calScale {
	case calWeek:
		return from.Format("02 Jan") + " – " + from.AddDate(0, 0, 6).Format("02 Jan 2006")
	case calMonth:
		return from.Format("January 2006")
	default:
		return from.Format("2006")
	}
}

// dayCell renders one day's box contents.
//
// Entries that do not fit are replaced by a "+N more" line rather than being
// dropped silently: a cell that shows two of five items and says nothing about
// the rest reads as a complete list.
func (a App) dayCell(day time.Time, entries []calEntry, width, height int, showTime bool) []string {
	return a.dayCellSel(day, entries, width, height, showTime, false)
}

// dayCellSel is dayCell with the cursor drawn on it.
func (a App) dayCellSel(day time.Time, entries []calEntry, width, height int, showTime, isCursor bool) []string {
	muted := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	text := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)
	event := lipgloss.NewStyle().Foreground(colorPurple).Background(colorPaneBg)
	late := lipgloss.NewStyle().Foreground(colorDanger).Background(colorPaneBg)

	today := startOfDay(a.now())
	head := day.Format("2")
	headStyle := muted
	if day.Equal(today) {
		head += " today"
		headStyle = lipgloss.NewStyle().Foreground(colorAccent).Background(colorPaneBg).Bold(true)
	}
	if isCursor {
		head = "▌" + head
		headStyle = headStyle.Background(colorPanel).Bold(true)
	}

	rows := []string{headStyle.Render(fitToWidth(head, width))}
	if height <= 1 {
		return rows
	}

	room := height - 1
	// One line is given back to the overflow marker whenever it is needed.
	shown := len(entries)
	if shown > room {
		shown = room - 1
		if shown < 0 {
			shown = 0
		}
	}
	// Keep the selected entry on screen: it is the one about to be edited, so
	// scrolling it out from under the cursor would be the worst row to hide.
	start := 0
	sel := -1
	if isCursor && len(entries) > 0 {
		sel = clamp(a.calEntrySel, 0, len(entries)-1)
		if sel >= shown {
			start = sel - shown + 1
		}
	}
	for i := start; i < start+shown && i < len(entries); i++ {
		e := entries[i]
		style := text
		switch {
		case e.overdue:
			style = late
		case e.kind == tlEvent:
			style = event
		}
		prefix := ""
		if i == sel {
			style = style.Background(colorPanel).Bold(true)
			prefix = "›"
		}
		rows = append(rows, style.Render(fitToWidth(prefix+e.label(showTime), width)))
	}
	if rest := len(entries) - shown; rest > 0 {
		rows = append(rows, muted.Render(fitToWidth(fmt.Sprintf("+%d more", rest), width)))
	}
	return rows
}

// renderWeek is seven day columns side by side.
func (a App) renderWeek(from time.Time, days map[string][]calEntry, width, height int) string {
	colWidth := width / 7
	if colWidth < 8 {
		// Too narrow for seven columns; fall back to a stacked agenda so the
		// content stays readable instead of being shaved to initials.
		return a.renderAgenda(from, from.AddDate(0, 0, 7), days, width, height)
	}

	cells := make([][]string, 7)
	cursor := a.selectedDay()
	for i := 0; i < 7; i++ {
		day := from.AddDate(0, 0, i)
		cells[i] = a.dayCellSel(day, days[day.Format("2006-01-02")], colWidth-1, height-1, true, day.Equal(cursor))
	}

	muted := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	var head []string
	for i := 0; i < 7; i++ {
		head = append(head, muted.Render(fitToWidth(weekdayNames[i], colWidth-1)))
	}

	rows := []string{strings.Join(head, " ")}
	rows = append(rows, joinCells(cells, colWidth, height-1)...)
	return strings.Join(rows, "\n")
}

// renderMonth is a week-per-row grid.
func (a App) renderMonth(from, to time.Time, days map[string][]calEntry, width, height int) string {
	colWidth := width / 7
	if colWidth < 8 {
		return a.renderAgenda(from, to, days, width, height)
	}

	// The grid starts on the Monday on or before the 1st.
	offset := (int(from.Weekday()) + 6) % 7
	gridStart := from.AddDate(0, 0, -offset)
	weeks := 0
	for d := gridStart; d.Before(to); d = d.AddDate(0, 0, 7) {
		weeks++
	}
	if weeks == 0 {
		weeks = 1
	}

	muted := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	var head []string
	for i := 0; i < 7; i++ {
		head = append(head, muted.Render(fitToWidth(weekdayNames[i], colWidth-1)))
	}
	rows := []string{strings.Join(head, " ")}

	cursor := a.selectedDay()
	cellHeight := (height - 1) / weeks
	if cellHeight < 2 {
		cellHeight = 2
	}
	for w := 0; w < weeks; w++ {
		cells := make([][]string, 7)
		for i := 0; i < 7; i++ {
			day := gridStart.AddDate(0, 0, w*7+i)
			entries := days[day.Format("2006-01-02")]
			if day.Before(from) || !day.Before(to) {
				// Days spilling in from the neighbouring months are shown as
				// empty rather than omitted, so the weekday columns stay
				// aligned all the way down.
				entries = nil
			}
			cells[i] = a.dayCellSel(day, entries, colWidth-1, cellHeight, false, day.Equal(cursor))
		}
		rows = append(rows, joinCells(cells, colWidth, cellHeight)...)
	}
	return strings.Join(rows, "\n")
}

// renderYear is twelve month summaries: a count per month, and the busiest
// days named.
//
// At this zoom no cell can hold an item's text, so the year view answers "when
// is it busy" rather than "what is on" — the counts are the content.
func (a App) renderYear(from time.Time, days map[string][]calEntry, width, height int) string {
	perMonth := make([]int, 12)
	busiest := make([]string, 12)
	busiestN := make([]int, 12)
	for key, entries := range days {
		day, err := time.Parse("2006-01-02", key)
		if err != nil {
			continue
		}
		m := int(day.Month()) - 1
		perMonth[m] += len(entries)
		if len(entries) > busiestN[m] {
			busiestN[m] = len(entries)
			busiest[m] = day.Format("02 Jan")
		}
	}

	muted := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	text := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)
	accent := lipgloss.NewStyle().Foreground(colorAccent).Background(colorPaneBg).Bold(true)

	cols := 3
	colWidth := width / cols
	if colWidth < 18 {
		cols, colWidth = 1, width
	}

	now := a.now()
	var rows []string
	var line []string
	for m := 0; m < 12; m++ {
		month := time.Date(from.Year(), time.Month(m+1), 1, 0, 0, 0, 0, from.Location())
		style := text
		if month.Year() == now.Year() && month.Month() == now.Month() {
			style = accent
		}
		label := fmt.Sprintf("%-10s %3d", month.Format("January"), perMonth[m])
		if busiestN[m] > 0 {
			label += muted.Render("") + fmt.Sprintf("  busiest %s", busiest[m])
		}
		line = append(line, style.Render(fitToWidth(label, colWidth-1)))
		if len(line) == cols {
			rows = append(rows, strings.Join(line, " "))
			line = nil
		}
	}
	if len(line) > 0 {
		rows = append(rows, strings.Join(line, " "))
	}
	if len(rows) > height {
		rows = rows[:height]
	}
	return strings.Join(rows, "\n")
}

// renderAgenda is the fallback when the terminal is too narrow for columns.
func (a App) renderAgenda(from, to time.Time, days map[string][]calEntry, width, height int) string {
	muted := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	text := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)

	var rows []string
	for d := from; d.Before(to) && len(rows) < height; d = d.AddDate(0, 0, 1) {
		entries := days[d.Format("2006-01-02")]
		if len(entries) == 0 {
			continue
		}
		rows = append(rows, muted.Render(fitToWidth(d.Format("Mon 02 Jan"), width)))
		for _, e := range entries {
			if len(rows) >= height {
				break
			}
			rows = append(rows, text.Render(fitToWidth("  "+e.label(true), width)))
		}
	}
	if len(rows) == 0 {
		rows = append(rows, muted.Render("Nothing in this period."))
	}
	return strings.Join(rows, "\n")
}

// joinCells lays out one row of day cells, padding each to the same height so
// the columns stay aligned.
func joinCells(cells [][]string, colWidth, height int) []string {
	blank := lipgloss.NewStyle().Background(colorPaneBg).Render(strings.Repeat(" ", maxInt(0, colWidth-1)))
	var rows []string
	for line := 0; line < height; line++ {
		var parts []string
		for _, cell := range cells {
			if line < len(cell) {
				parts = append(parts, cell[line])
			} else {
				parts = append(parts, blank)
			}
		}
		rows = append(rows, strings.Join(parts, " "))
	}
	return rows
}
