// Package obsidian drives the official Obsidian CLI.
//
// This is the write path, and only the write path. Anything that must go
// through plugin logic — a Jira workflow transition, a comment, a fetch —
// has to be executed inside the running app, because that is where the plugins
// live. Reads deliberately do not come through here: see internal/para.
//
// Two constraints shape this package:
//
//   - The CLI requires the desktop app to be running, and LAUNCHES IT if it is
//     not. A background timer must therefore never call into this package, or
//     it will pop Obsidian open on a locked or headless session.
//   - The binary must be resolved from PATH. Obsidian ships a copy inside its
//     AppImage, but that unpacks to /tmp/.mount_Obsidia<random>/ and the
//     directory name changes on every launch, so it is never a valid target.
package obsidian

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrUnavailable means the CLI could not be found or is not enabled. The CLI
// ships disabled; a user turns it on in Settings > General > Advanced, which
// registers it into ~/.local/bin.
var ErrUnavailable = errors.New("obsidian CLI unavailable")

// DefaultTimeout bounds a single CLI call. Commands that open a modal in the
// GUI return once the command is dispatched, not once the human is done, so
// this only needs to cover dispatch.
const DefaultTimeout = 20 * time.Second

type Client struct {
	bin     string
	vault   string
	Timeout time.Duration
}

// Discover locates the CLI on PATH, then in the standard install location.
func Discover() (string, error) {
	if p, err := exec.LookPath("obsidian"); err == nil {
		return p, nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, ".local", "bin", "obsidian")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", ErrUnavailable
}

// New builds a client for a vault. It returns a client even when the CLI is
// missing, so the UI can show why an action is unavailable rather than hiding
// the keybinding.
func New(vault string) *Client {
	bin, _ := Discover()
	return &Client{bin: bin, vault: vault, Timeout: DefaultTimeout}
}

// Available reports whether calls have any chance of succeeding.
func (c *Client) Available() bool { return c != nil && c.bin != "" }

// Unavailable explains, in one line, why actions are disabled.
func (c *Client) Unavailable() string {
	if c.Available() {
		return ""
	}
	return "Obsidian CLI not found — enable it in Obsidian: Settings > General > Advanced"
}

func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	if !c.Available() {
		return "", ErrUnavailable
	}
	if c.vault != "" {
		args = append(args, "vault="+c.vault)
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.bin, args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return text, fmt.Errorf("obsidian %s: timed out after %s", strings.Join(args, " "), timeout)
		}
		if text != "" {
			return text, fmt.Errorf("obsidian %s: %s", strings.Join(args, " "), firstLine(text))
		}
		return text, fmt.Errorf("obsidian %s: %w", strings.Join(args, " "), err)
	}
	return text, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Command runs any registered command by its palette ID, which is how every
// plugin action is reached (for example "jira-sync:batch-fetch-issues-jira").
func (c *Client) Command(ctx context.Context, id string) (string, error) {
	return c.run(ctx, "command", "id="+id)
}

// Open focuses a note. Several plugin commands read the key from the ACTIVE
// file's frontmatter rather than taking an argument, so acting on a specific
// ticket means opening it first.
func (c *Client) Open(ctx context.Context, path string) (string, error) {
	return c.run(ctx, "open", "file="+path)
}

// OpenThenCommand is the two-step an active-file command needs.
func (c *Client) OpenThenCommand(ctx context.Context, path, id string) (string, error) {
	if _, err := c.Open(ctx, path); err != nil {
		return "", err
	}
	return c.Command(ctx, id)
}

// Eval runs JavaScript against the app object. This reaches things the command
// surface does not expose, at the cost of depending on a plugin's internals,
// which are minified and change between releases — prefer Command.
func (c *Client) Eval(ctx context.Context, js string) (string, error) {
	return c.run(ctx, "eval", "code="+js)
}

// Cmd is one entry from the command registry.
type Cmd struct {
	ID   string
	Name string
}

// Commands lists registered command IDs, optionally filtered by prefix.
func (c *Client) Commands(ctx context.Context, filter string) ([]Cmd, error) {
	args := []string{"commands"}
	if filter != "" {
		args = append(args, "filter="+filter)
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var cmds []Cmd
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// An ID is namespaced "<plugin>:<command>"; anything after it on the
		// line is the human-readable name.
		id, name, _ := strings.Cut(line, " ")
		if !strings.Contains(id, ":") {
			continue
		}
		cmds = append(cmds, Cmd{ID: id, Name: strings.TrimSpace(name)})
	}
	return cmds, nil
}
