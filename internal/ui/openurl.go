package ui

import (
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
)

// openInBrowser launches url in the system's default browser, indirected the
// same way desktopSend is: real OS calls in production, a stub in tests.
var openInBrowser = func(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		// "start" is a cmd builtin, not an executable, and its first
		// argument is taken as the window title — an empty one keeps a URL
		// containing spaces or special characters from being misread as it.
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// openURLMsg reports whether launching the browser succeeded.
type openURLMsg struct {
	url string
	err error
}

// openURLCmd opens url without blocking the UI on the external process.
func openURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		return openURLMsg{url: url, err: openInBrowser(url)}
	}
}

// openTicketLink opens the selected ticket's link — the Jira issue itself,
// falling back to its PR — in the default browser.
//
// Jira is preferred because every ticket has one; a PR link is only ever set
// once work has started, so it is the exception rather than the common case.
func (a App) openTicketLink() (tea.Model, tea.Cmd) {
	t, ok := a.selectedTicket()
	if !ok {
		a.setErr("no ticket selected")
		return a, nil
	}
	url := t.Link
	if url == "" {
		url = t.PRLink
	}
	if url == "" {
		a.setErr("this ticket has no link set")
		return a, nil
	}
	a.setStatus("opening " + url)
	return a, openURLCmd(url)
}
