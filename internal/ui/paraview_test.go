package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"taskii/internal/para"
)

func fixtureVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Tickets/AAA-1 In Progress live.md", "---\nkey: AAA-1\nsummary: normalize cities\nstatus: In Progress\narea: QCOM\nduedate: 2026-08-25\n---\n\n## Notes & Sub-tasks\n- [x] added middleware\n- [ ] check zepto spider\n")
	write("Tickets/AAA-2 Done stranded.md", "---\nkey: AAA-2\nsummary: stranded one\nstatus: Done\narea: \n---\n")
	write("Areas/QCOM/Info.md", "---\nstatus: active\n---\n")
	return root
}

// newTestApp builds an app against a fixture vault with the index already
// delivered, as it would be after the initial scan completes.
func newTestApp(t *testing.T, width, height int) App {
	t.Helper()
	root := fixtureVault(t)
	a := NewApp(Options{Mock: true, Vault: root, Location: time.UTC})
	a.loc = time.UTC

	idx, err := para.Scan(root, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m, _ = m.(App).Update(indexMsg{idx: idx})
	return m.(App)
}

func press(t *testing.T, a App, keys ...string) App {
	t.Helper()
	var m tea.Model = a
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter", "tab", "esc", "up", "down", "space":
			msg = tea.KeyMsg{Type: map[string]tea.KeyType{
				"enter": tea.KeyEnter, "tab": tea.KeyTab, "esc": tea.KeyEsc,
				"up": tea.KeyUp, "down": tea.KeyDown, "space": tea.KeySpace,
			}[k]}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		m, _ = m.(App).Update(msg)
	}
	return m.(App)
}

func TestParaViewShowsAreasAndTickets(t *testing.T) {
	a := press(t, newTestApp(t, 150, 40), "2")
	if a.view != viewPARA {
		t.Fatalf("view = %v, want PARA", a.view)
	}
	out := a.View()
	for _, want := range []string{"All tickets", "QCOM", "AAA-1", "normalize cities"} {
		if !strings.Contains(out, want) {
			t.Errorf("PARA view missing %q", want)
		}
	}
}

// The stranded ticket is the one the vault's own automation cannot file, so it
// must be visible without hunting for it.
func TestParaViewSurfacesUnfiled(t *testing.T) {
	a := press(t, newTestApp(t, 150, 40), "2")
	if !strings.Contains(a.View(), "Unfiled") {
		t.Errorf("Unfiled row absent:\n%s", a.View())
	}
}

func TestDetailPaneShowsCheckboxes(t *testing.T) {
	// tab to the ticket list, then into the detail pane.
	a := press(t, newTestApp(t, 150, 40), "2", "tab", "tab")
	out := a.View()
	if !strings.Contains(out, "check zepto spider") {
		t.Errorf("detail pane missing the task line:\n%s", out)
	}
	if !strings.Contains(out, "[x]") || !strings.Contains(out, "[ ]") {
		t.Error("detail pane should show both checked and unchecked tasks")
	}
}

// A narrow terminal must drop panes rather than squeeze three columns into
// unreadable slivers.
func TestParaViewAdaptsToNarrowTerminals(t *testing.T) {
	wide := press(t, newTestApp(t, 150, 40), "2").paraGeometry()
	if wide.detailWidth == 0 {
		t.Error("a wide terminal should show the detail pane")
	}
	medium := press(t, newTestApp(t, 100, 40), "2").paraGeometry()
	if medium.detailWidth != 0 || medium.treeWidth == 0 {
		t.Error("a medium terminal should drop the detail pane but keep the tree")
	}
	narrow := press(t, newTestApp(t, 70, 40), "2").paraGeometry()
	if narrow.treeWidth != 0 || narrow.detailWidth != 0 {
		t.Error("a narrow terminal should show a single pane")
	}
}

func TestCalendarViewListsDueDates(t *testing.T) {
	a := press(t, newTestApp(t, 150, 40), "3")
	if a.view != viewCalendar {
		t.Fatalf("view = %v, want Calendar", a.view)
	}
	out := a.View()
	if !strings.Contains(out, "normalize cities") {
		t.Errorf("calendar missing the dated ticket:\n%s", out)
	}
	if !strings.Contains(out, "25 Aug") {
		t.Errorf("calendar missing the due day:\n%s", out)
	}
}

func TestViewSwitchingRoundTrips(t *testing.T) {
	a := newTestApp(t, 150, 40)
	if got := press(t, a, "2", "3", "1").view; got != viewDashboard {
		t.Errorf("view = %v, want Dashboard", got)
	}
}

func TestSettingsOverlayOpensAndValidates(t *testing.T) {
	a := press(t, newTestApp(t, 150, 40), ",")
	if !a.settingsUI.open {
		t.Fatal("settings did not open")
	}
	if !strings.Contains(a.View(), "Timezone") {
		t.Errorf("settings missing the timezone field:\n%s", a.View())
	}

	// Enter edit mode on Timezone and submit something invalid.
	a = press(t, a, "enter")
	if !a.settingsUI.editing {
		t.Fatal("did not enter edit mode")
	}
	a.settingsUI.input.SetValue("Not/AZone")
	a = press(t, a, "enter")
	if a.settingsUI.err == "" {
		t.Error("an unknown timezone should be rejected in the overlay")
	}
	if a.tzSetting == "Not/AZone" {
		t.Error("invalid timezone was persisted")
	}
}

func TestSettingsAcceptsValidTimezone(t *testing.T) {
	if _, err := time.LoadLocation("Europe/Berlin"); err != nil {
		t.Skip("tzdata unavailable")
	}
	a := press(t, newTestApp(t, 150, 40), ",", "enter")
	a.settingsUI.input.SetValue("Europe/Berlin")
	a = press(t, a, "enter")
	if a.settingsUI.err != "" {
		t.Fatalf("valid timezone rejected: %s", a.settingsUI.err)
	}
	if a.tzSetting != "Europe/Berlin" {
		t.Errorf("tzSetting = %q, want Europe/Berlin", a.tzSetting)
	}
	if a.loc.String() != "Europe/Berlin" {
		t.Errorf("location = %v, want Europe/Berlin", a.loc)
	}
}

// The dashboard must keep working exactly as before when no vault is present.
func TestDashboardStillRendersWithoutVault(t *testing.T) {
	a := NewApp(Options{Mock: true, Location: time.UTC})
	m, _ := a.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	out := m.(App).View()
	if !strings.Contains(out, "Today") {
		t.Errorf("dashboard did not render:\n%s", out)
	}
}

func TestParaViewExplainsMissingVault(t *testing.T) {
	a := NewApp(Options{Mock: true, Location: time.UTC})
	m, _ := a.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	got := press(t, m.(App), "2")
	if !strings.Contains(got.View(), "No vault configured") {
		t.Errorf("expected an explanation, got:\n%s", got.View())
	}
}

// The detail pane pads each label to the full pane width, which pushes the
// value off the right edge — every field renders as an empty label.
func TestDetailPaneShowsFieldValues(t *testing.T) {
	a := press(t, newTestApp(t, 150, 40), "2")
	out := a.View()
	for _, want := range []string{"In Progress", "QCOM"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail pane does not show field value %q", want)
		}
	}
}

// Quick-add captures keys but nothing renders the input, so typing looks like
// it does nothing at all.
func TestQuickAddShowsWhatIsTyped(t *testing.T) {
	a := press(t, newTestApp(t, 150, 40), "2", "a")
	if a.mode != modeVaultAdding {
		t.Fatalf("mode = %v, want modeVaultAdding", a.mode)
	}
	a = press(t, a, "h", "e", "l", "l", "o")
	if got := a.input.Value(); got != "hello" {
		t.Fatalf("input value = %q, want \"hello\"", got)
	}
	if !strings.Contains(a.View(), "hello") {
		t.Errorf("typed text is not rendered anywhere:\n%s", a.View())
	}
}

// Reindex must pick up changes made on disk while the app is open — Obsidian,
// its plugins and a Jira fetch all rewrite these notes behind our back.
func TestReindexPicksUpDiskChanges(t *testing.T) {
	root := fixtureVault(t)
	a := NewApp(Options{Mock: true, Vault: root, Location: time.UTC})
	a.loc = time.UTC
	m, _ := a.Update(tea.WindowSizeMsg{Width: 150, Height: 40})

	// Deliver the initial index the way Init's command would.
	idx, err := para.Scan(root, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.(App).Update(indexMsg{idx: idx})
	app := press(t, m.(App), "2")
	if strings.Contains(app.View(), "brand new task") {
		t.Fatal("fixture already contains the new task")
	}

	// Change the note on disk, then reindex.
	path := filepath.Join(root, "Tickets", "AAA-1 In Progress live.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, []byte("- [ ] brand new task\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	next, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("r produced no reindex command")
	}
	msg := cmd()
	got, ok := msg.(indexMsg)
	if !ok {
		t.Fatalf("reindex returned %T, want indexMsg", msg)
	}
	if got.err != nil {
		t.Fatal(got.err)
	}
	final, _ := next.(App).Update(got)
	app = final.(App)

	// Move focus to the detail pane so the task list is on screen.
	app = press(t, app, "tab", "tab")
	if !strings.Contains(app.View(), "brand new task") {
		t.Errorf("reindex did not surface the new task:\n%s", app.View())
	}
}
