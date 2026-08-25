package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"taskii/internal/para"
)

// linkedVault builds a fixture with a ticket carrying both a Jira link and a
// PR link, since the shared fixture used elsewhere in this package has
// neither.
func linkedVaultApp(t *testing.T, width, height int) App {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "Tickets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nkey: LNK-1\nsummary: has links\nstatus: In Progress\n" +
		"link: https://data-impact.atlassian.net/browse/LNK-1\n" +
		"pr_link: https://github.com/example/repo/pull/42\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "LNK-1 In Progress has links.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewApp(Options{Mock: true, Vault: root, Location: time.UTC})
	a.loc = time.UTC
	m, _ := a.Update(tea.WindowSizeMsg{Width: width, Height: height})
	idx, err := para.Scan(root, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.(App).Update(indexMsg{idx: idx})
	return press(t, m.(App), "2", "tab") // PARA view, ticket list focused
}

func TestDetailPaneShowsClickableLinkFields(t *testing.T) {
	a := linkedVaultApp(t, 150, 40)
	out := a.View()

	if !strings.Contains(out, "Link:") || !strings.Contains(out, "PR:") {
		t.Errorf("Link/PR fields are not shown:\n%s", out)
	}
	if !strings.Contains(out, ansi.SetHyperlink("https://data-impact.atlassian.net/browse/LNK-1")) {
		t.Errorf("the Jira link is not clickable:\n%s", out)
	}
	if !strings.Contains(out, ansi.SetHyperlink("https://github.com/example/repo/pull/42")) {
		t.Errorf("the PR link is not clickable:\n%s", out)
	}
}

func TestOpenLinkKeyPrefersJiraOverPR(t *testing.T) {
	var opened string
	old := openInBrowser
	openInBrowser = func(url string) error { opened = url; return nil }
	defer func() { openInBrowser = old }()

	a := linkedVaultApp(t, 150, 40)
	next, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	a = next.(App)
	runCmd(t, cmd)

	if opened != "https://data-impact.atlassian.net/browse/LNK-1" {
		t.Errorf("opened %q, want the Jira link", opened)
	}
	if a.err != "" {
		t.Errorf("l reported: %s", a.err)
	}
}

// With no Jira link, the PR link is the fallback.
func TestOpenLinkFallsBackToPR(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Tickets")
	os.MkdirAll(dir, 0o755)
	body := "---\nkey: LNK-2\nsummary: pr only\nstatus: In Progress\n" +
		"pr_link: https://github.com/example/repo/pull/7\n---\n"
	os.WriteFile(filepath.Join(dir, "LNK-2 In Progress pr only.md"), []byte(body), 0o644)

	a := NewApp(Options{Mock: true, Vault: root, Location: time.UTC})
	a.loc = time.UTC
	m, _ := a.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	idx, err := para.Scan(root, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.(App).Update(indexMsg{idx: idx})
	a = press(t, m.(App), "2", "tab")

	var opened string
	old := openInBrowser
	openInBrowser = func(url string) error { opened = url; return nil }
	defer func() { openInBrowser = old }()

	next, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	a = next.(App)
	runCmd(t, cmd)

	if opened != "https://github.com/example/repo/pull/7" {
		t.Errorf("opened %q, want the PR link", opened)
	}
}

func TestOpenLinkReportsWhenTicketHasNone(t *testing.T) {
	a := press(t, newTestApp(t, 150, 40), "2", "tab") // fixture ticket has no link
	old := openInBrowser
	called := false
	openInBrowser = func(url string) error { called = true; return nil }
	defer func() { openInBrowser = old }()

	a = press(t, a, "l")
	if called {
		t.Error("a browser was launched for a ticket with no link")
	}
	if a.err == "" {
		t.Error("no explanation was given")
	}
}

func TestOpenLinkReportsAFailure(t *testing.T) {
	old := openInBrowser
	openInBrowser = func(url string) error { return os.ErrNotExist }
	defer func() { openInBrowser = old }()

	a := linkedVaultApp(t, 150, 40)
	next, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	a = next.(App)
	if cmd == nil {
		t.Fatal("no command was returned")
	}
	// openURLCmd returns a plain (non-batched) Cmd, so its result is fed back
	// through Update directly rather than via runCmd, which only recurses
	// into batches.
	final, _ := a.Update(cmd())
	a = final.(App)

	if a.err == "" {
		t.Error("a failed launch was not reported")
	}
}
