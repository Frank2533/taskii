package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"taskii/internal/deadline"
	"taskii/internal/para"
)

// view is the top-level screen. Panes are composed per view rather than all
// living in one grid: the dashboard's four panes plus a PARA tree, a ticket
// list, a detail pane and a calendar would not fit any terminal usefully.
type view int

const (
	viewDashboard view = iota
	viewPARA
	viewCalendar

	viewCount = 3
)

func (v view) String() string {
	switch v {
	case viewPARA:
		return "PARA"
	case viewCalendar:
		return "Calendar"
	default:
		return "Dashboard"
	}
}

// paraPane is the focus target within the PARA view.
type paraPane int

const (
	paraTree paraPane = iota
	paraList
	paraDetail

	paraPaneCount = 3
)

// rowKind distinguishes what a tree row selects.
type rowKind int

const (
	rowAll rowKind = iota
	rowUnfiled
	rowArea
	rowProject
	rowLocalTasks
)

// treeRow is one line in the navigator.
type treeRow struct {
	kind    rowKind
	label   string
	area    string
	project int // index into idx.Projects when kind is rowProject
	count   int
}

// vaultState is the PARA view's selection and scroll state.
type vaultState struct {
	pane paraPane

	treeSel    int
	treeScroll int

	listSel    int
	listScroll int

	detailSel    int
	detailScroll int
}

// treeRows builds the navigator: everything, the stranded tickets, one row per
// area, then projects.
func (a App) treeRows() []treeRow {
	if a.idx == nil {
		return nil
	}
	rows := []treeRow{{kind: rowAll, label: "All tickets", count: len(a.idx.Tickets)}}
	if n := len(a.idx.Unfiled()); n > 0 {
		// Only shown when it has contents — an always-present empty row would
		// train the eye to ignore it, and this is the row that matters.
		rows = append(rows, treeRow{kind: rowUnfiled, label: "Unfiled", count: n})
	}
	if n := len(a.idx.OpenLocalTasks()); n > 0 {
		rows = append(rows, treeRow{kind: rowLocalTasks, label: "Local tasks", count: n})
	}
	for _, name := range a.idx.AreaNames() {
		rows = append(rows, treeRow{
			kind:  rowArea,
			label: name,
			area:  name,
			count: len(a.idx.InArea(name)),
		})
	}
	for i, p := range a.idx.Projects {
		rows = append(rows, treeRow{
			kind:    rowProject,
			label:   p.Title,
			project: i,
			count:   len(a.idx.ForProject(p)),
		})
	}
	return rows
}

// visibleTickets is the ticket list for the selected navigator row.
func (a App) visibleTickets() []para.Ticket {
	if a.idx == nil {
		return nil
	}
	rows := a.treeRows()
	if len(rows) == 0 {
		return nil
	}
	sel := clamp(a.vault.treeSel, 0, len(rows)-1)
	row := rows[sel]
	switch row.kind {
	case rowLocalTasks:
		return localTasksAsTickets(a.idx.OpenLocalTasks())
	case rowUnfiled:
		return a.idx.Unfiled()
	case rowArea:
		return a.idx.InArea(row.area)
	case rowProject:
		if row.project < len(a.idx.Projects) {
			return a.idx.ForProject(a.idx.Projects[row.project])
		}
		return nil
	default:
		return a.idx.Tickets
	}
}

// selectedTicket is the ticket the detail pane and every ticket action act on.
func (a App) selectedTicket() (para.Ticket, bool) {
	list := a.visibleTickets()
	if len(list) == 0 {
		return para.Ticket{}, false
	}
	return list[clamp(a.vault.listSel, 0, len(list)-1)], true
}

// selectedCheckbox is the task line the detail pane has highlighted.
func (a App) selectedCheckbox() (para.Checkbox, bool) {
	t, ok := a.selectedTicket()
	if !ok || len(t.Checkboxes) == 0 {
		return para.Checkbox{}, false
	}
	return t.Checkboxes[clamp(a.vault.detailSel, 0, len(t.Checkboxes)-1)], true
}

// localTasksAsTickets adapts local tasks for the ticket list and detail panes.
//
// They are shown through the same panes rather than given their own, because
// everything those panes display — a title, a status, a set of task lines —
// a local task also has. The adapter is display-only; nothing is written back
// through it.
func localTasksAsTickets(tasks []para.LocalTask) []para.Ticket {
	out := make([]para.Ticket, 0, len(tasks))
	for _, t := range tasks {
		status := "Open"
		if t.Done {
			status = "Done"
		}
		out = append(out, para.Ticket{
			Summary:    t.Title,
			Status:     status,
			IssueType:  "Local task",
			Path:       t.Path,
			Due:        t.Due,
			HasDue:     t.HasDue,
			Checkboxes: t.Checkboxes,
		})
	}
	return out
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// paraGeometry splits the body between the three panes, dropping panes on
// narrow terminals rather than squeezing all three into unreadable columns.
type paraGeometry struct {
	treeWidth   int
	listWidth   int
	detailWidth int
	height      int
}

func (a App) paraGeometry() paraGeometry {
	height := a.height - a.chromeLines()
	if a.mode == modeVaultAdding || a.mode == modeTicketNote {
		// The input occupies a line below the panes.
		height--
	}
	if height < 3 {
		height = 3
	}
	w := a.width

	switch {
	case w < 80:
		// One pane: whichever is focused.
		return paraGeometry{listWidth: w, height: height}
	case w < 120:
		// Two panes: the detail pane goes first, since the list carries the
		// identifying text.
		tree := w / 3
		return paraGeometry{treeWidth: tree, listWidth: w - tree, height: height}
	default:
		tree := w / 5
		detail := w / 3
		return paraGeometry{treeWidth: tree, detailWidth: detail, listWidth: w - tree - detail, height: height}
	}
}

func (a App) renderPara() string {
	g := a.paraGeometry()

	if a.vaultStatusLine() != "" {
		body := lipgloss.NewStyle().Foreground(colorWarning).Render(a.vaultStatusLine())
		return renderPane("PARA", body, true, a.width, g.height)
	}

	rows := a.treeRows()
	tickets := a.visibleTickets()

	// Narrow terminals show a single pane so the content stays legible.
	if g.treeWidth == 0 {
		switch a.vault.pane {
		case paraTree:
			return a.withQuickAdd(renderPane("Navigator", a.renderTree(rows, g.listWidth-4, g.height-2), true, g.listWidth, g.height), g)
		case paraDetail:
			return a.withQuickAdd(renderPane(a.detailTitle(), a.renderDetail(g.listWidth-4, g.height-2), true, g.listWidth, g.height), g)
		default:
			return a.withQuickAdd(renderPane(a.listTitle(rows), a.renderTicketList(tickets, g.listWidth-4, g.height-2), true, g.listWidth, g.height), g)
		}
	}

	panes := []string{
		renderPane("Navigator", a.renderTree(rows, g.treeWidth-4, g.height-2), a.vault.pane == paraTree, g.treeWidth, g.height),
		renderPane(a.listTitle(rows), a.renderTicketList(tickets, g.listWidth-4, g.height-2), a.vault.pane == paraList, g.listWidth, g.height),
	}
	if g.detailWidth > 0 {
		panes = append(panes, renderPane(a.detailTitle(), a.renderDetail(g.detailWidth-4, g.height-2), a.vault.pane == paraDetail, g.detailWidth, g.height))
	}
	return a.withQuickAdd(lipgloss.JoinHorizontal(lipgloss.Top, panes...), g)
}

// withQuickAdd appends the task input beneath the panes.
//
// It is rendered as a full-width line rather than inside a pane because the
// panes are fixed-size blocks and which of them is on screen depends on the
// terminal width — a prompt tucked into the detail pane would simply vanish on
// a narrow terminal, which is what made typing look like it did nothing.
func (a App) withQuickAdd(body string, g paraGeometry) string {
	if a.mode != modeVaultAdding && a.mode != modeTicketNote {
		return body
	}
	target := ""
	if t, ok := a.selectedTicket(); ok {
		target = t.Key
	}
	prompt := inputPromptStyle.Render("+ ")
	section := quickAddHeading
	if a.mode == modeTicketNote {
		section = worklogHeading
	}
	label := lipgloss.NewStyle().Foreground(colorMuted).Render(target + " " + section + "  ")

	a.input.TextStyle = lipgloss.NewStyle().Foreground(colorText)
	a.input.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent)
	a.input.Cursor.Style = lipgloss.NewStyle().Foreground(colorText)
	a.input.Width = 0

	return lipgloss.JoinVertical(lipgloss.Left, body, prompt+label+a.input.View())
}

// vaultStatusLine explains why the vault views are empty, if they are.
func (a App) vaultStatusLine() string {
	switch {
	case a.vaultPath == "":
		return "No vault configured. Pass -vault, or set one with , (settings)."
	case a.idxErr != nil:
		return "Could not read the vault: " + a.idxErr.Error()
	case a.idx == nil:
		return "Indexing the vault..."
	default:
		return ""
	}
}

func (a App) listTitle(rows []treeRow) string {
	if len(rows) == 0 {
		return "Tickets"
	}
	row := rows[clamp(a.vault.treeSel, 0, len(rows)-1)]
	return fmt.Sprintf("%s (%d)", row.label, row.count)
}

func (a App) detailTitle() string {
	if t, ok := a.selectedTicket(); ok && t.Key != "" {
		return t.Key
	}
	return "Detail"
}

func (a App) renderTree(rows []treeRow, width, height int) string {
	if len(rows) == 0 {
		return lipgloss.NewStyle().Foreground(colorMuted).Render("nothing indexed")
	}
	sel := clamp(a.vault.treeSel, 0, len(rows)-1)
	start := scrollWindow(sel, a.vault.treeScroll, height, len(rows))

	var out []string
	var lastKind rowKind = -1
	for i := start; i < len(rows) && len(out) < height; i++ {
		r := rows[i]
		if lastKind != -1 && r.kind != lastKind && len(out) < height-1 {
			out = append(out, "")
		}
		lastKind = r.kind

		label := fmt.Sprintf("%s %d", r.label, r.count)
		style := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)
		if r.kind == rowUnfiled {
			// Stranded tickets are the one thing here that needs action.
			style = style.Foreground(colorWarning)
		}
		if r.kind == rowProject || r.kind == rowLocalTasks {
			style = style.Foreground(colorPurple)
		}
		prefix := "  "
		if i == sel {
			prefix = "> "
			style = style.Bold(true)
			if a.vault.pane == paraTree {
				style = style.Foreground(colorAccent)
			}
		}
		out = append(out, style.Render(fitToWidth(prefix+label, width)))
	}
	return strings.Join(out, "\n")
}

func (a App) renderTicketList(tickets []para.Ticket, width, height int) string {
	if len(tickets) == 0 {
		return lipgloss.NewStyle().Foreground(colorMuted).Render("no tickets here")
	}
	sel := clamp(a.vault.listSel, 0, len(tickets)-1)
	start := scrollWindow(sel, a.vault.listScroll, height, len(tickets))

	var out []string
	for i := start; i < len(tickets) && len(out) < height; i++ {
		t := tickets[i]
		style := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)
		switch {
		case t.Stranded():
			style = style.Foreground(colorWarning)
		case t.Closed():
			style = style.Foreground(colorMuted)
		}
		prefix := "  "
		if i == sel {
			prefix = "> "
			style = style.Bold(true)
			if a.vault.pane == paraList {
				style = style.Foreground(colorAccent)
			}
		}
		open := 0
		for _, c := range t.Checkboxes {
			if !c.Done {
				open++
			}
		}
		suffix := ""
		if open > 0 {
			suffix = fmt.Sprintf("  [%d]", open)
		}
		out = append(out, style.Render(fitToWidth(prefix+t.Title()+suffix, width)))
	}
	return strings.Join(out, "\n")
}

func (a App) renderDetail(width, height int) string {
	t, ok := a.selectedTicket()
	if !ok {
		return lipgloss.NewStyle().Foreground(colorMuted).Render("nothing selected")
	}
	muted := lipgloss.NewStyle().Foreground(colorMuted).Background(colorPaneBg)
	text := lipgloss.NewStyle().Foreground(colorText).Background(colorPaneBg)

	var out []string
	add := func(s string) {
		if len(out) < height {
			out = append(out, s)
		}
	}

	add(text.Bold(true).Render(fitToWidth(t.Summary, width)))
	// Labels are padded to a fixed column, NOT to the pane width: fitting the
	// label to the full width pushed every value off the right edge, so each
	// field rendered as a label with nothing after it.
	const labelCol = 10
	field := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		label = fmt.Sprintf("%-*s", labelCol, label+":")
		room := width - labelCol
		if room < 1 {
			return
		}
		add(muted.Render(label) + text.Render(fitToWidth(value, room)))
	}
	field("Status", t.Status)
	field("Area", t.Area)
	field("Type", t.IssueType)
	field("Priority", t.Priority)
	field("Epic", t.EpicLink)
	field("Sprint", t.Sprint)

	// Rendered in accent colour with an OSC 8 wrapper, so a terminal that
	// understands hyperlinks makes the whole field clickable — and l opens it
	// directly for one that does not.
	linkField := func(label, url string) {
		if strings.TrimSpace(url) == "" {
			return
		}
		lbl := fmt.Sprintf("%-*s", labelCol, label+":")
		room := width - labelCol
		if room < 1 {
			return
		}
		value := lipgloss.NewStyle().Foreground(colorAccent).Background(colorPaneBg).
			Underline(true).Render(fitToWidth(url, room))
		add(muted.Render(lbl) + hyperlinkField(value, url))
	}
	linkField("Link", t.Link)
	linkField("PR", t.PRLink)
	if t.HasDue {
		field("Due", t.Due.Format("Mon 02 Jan 2006"))
	}
	if t.Stranded() {
		add(lipgloss.NewStyle().Foreground(colorWarning).Background(colorPaneBg).
			Render(fitToWidth("closed with no area - press u to file it", width)))
	}
	if pending := a.pendingTimeFor(t.Key); pending != "" {
		field("Tracked", pending+" not yet logged")
	}

	if len(t.Checkboxes) > 0 {
		add("")
		add(muted.Render("Tasks"))
		sel := clamp(a.vault.detailSel, 0, len(t.Checkboxes)-1)
		start := scrollWindow(sel, a.vault.detailScroll, height-len(out), len(t.Checkboxes))
		for i := start; i < len(t.Checkboxes) && len(out) < height; i++ {
			c := t.Checkboxes[i]
			box := "[ ]"
			style := text
			if c.Done {
				box = "[x]"
				style = muted.Strikethrough(true)
			}
			prefix := "  "
			if i == sel {
				prefix = "> "
				if a.vault.pane == paraDetail {
					style = style.Foreground(colorAccent).Bold(true)
				}
			}
			label := c.Text
			if c.HasDue {
				label += "  ⌛ " + deadline.Short(c.Due, a.now())
			}
			add(style.Render(fitToWidth(prefix+box+" "+label, width)))
		}
	}
	return strings.Join(out, "\n")
}

// scrollWindow returns the first index to render so that sel stays visible.
func scrollWindow(sel, current, height, total int) int {
	if height <= 0 || total <= height {
		return 0
	}
	start := current
	if sel < start {
		start = sel
	}
	if sel >= start+height {
		start = sel - height + 1
	}
	if start > total-height {
		start = total - height
	}
	if start < 0 {
		start = 0
	}
	return start
}
