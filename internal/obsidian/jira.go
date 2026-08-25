package obsidian

import (
	"context"
	"fmt"
	"strings"
)

// Command IDs registered by the jira-sync plugin, read from its bundle. The
// namespace is the plugin's manifest id.
const (
	CmdJiraFetchBatch    = "jira-sync:batch-fetch-issues-jira"
	CmdJiraFetchByKey    = "jira-sync:get-issue-jira-key"
	CmdJiraFetchActive   = "jira-sync:get-issue-jira"
	CmdJiraUpdateIssue   = "jira-sync:update-issue-jira"
	CmdJiraUpdateStatus  = "jira-sync:update-issue-status-jira"
	CmdJiraAddComment    = "jira-sync:add-comment-jira"
	CmdJiraWorklogManual = "jira-sync:update-work-log-jira-manually"
	CmdJiraWorklogBatch  = "jira-sync:update-work-log-jira-batch"
	CmdJiraRebuildCache  = "jira-sync:rebuild-issue-cache"

	CmdRenameFromProps = "properties-filename:rename-active-file"
)

// Availability describes whether a command can run right now.
type Availability int

const (
	// AvailUnknown means the check itself could not be performed.
	AvailUnknown Availability = iota
	// AvailMissing means no command with that ID is registered — the plugin
	// is absent, disabled, or has renamed it.
	AvailMissing
	// AvailBlocked means the command exists but its own precondition check
	// currently fails.
	AvailBlocked
	// AvailReady means it will run.
	AvailReady
)

func (a Availability) String() string {
	switch a {
	case AvailMissing:
		return "missing"
	case AvailBlocked:
		return "blocked"
	case AvailReady:
		return "ready"
	default:
		return "unknown"
	}
}

// Check reports whether a command can run.
//
// This exists because an unavailable command is otherwise indistinguishable
// from a no-op: the CLI dispatches it, nothing happens, and nothing is
// reported. Obsidian's own command list hides commands whose precondition
// fails, so listing is not enough to tell "not installed" from "not right now"
// — the distinction that decides whether the user should fix their setup or
// just select a different note.
func (c *Client) Check(ctx context.Context, id string) (Availability, error) {
	js := fmt.Sprintf(
		"(()=>{const c=app.commands.commands[%q];"+
			"if(!c)return 'missing';"+
			"if(!c.checkCallback)return 'ready';"+
			"return c.checkCallback(true)?'ready':'blocked';})()", id)
	out, err := c.Eval(ctx, js)
	if err != nil {
		return AvailUnknown, err
	}
	switch {
	case strings.Contains(out, "missing"):
		return AvailMissing, nil
	case strings.Contains(out, "blocked"):
		return AvailBlocked, nil
	case strings.Contains(out, "ready"):
		return AvailReady, nil
	default:
		return AvailUnknown, nil
	}
}

// BlockedHint explains the most common reason a jira-sync command is blocked.
//
// Every one of the plugin's commands tests the ACTIVE connection before
// anything else, so a connection selected but left without a URL disables all
// of them at once — silently, since a blocked command simply vanishes from the
// palette rather than reporting why.
const BlockedHint = "unavailable right now — check jira-sync's active connection has a URL, and that the note has a `key` property"

// runChecked opens the note, verifies the command can run, then runs it.
func (c *Client) runChecked(ctx context.Context, notePath, id string) (string, error) {
	if notePath != "" {
		if _, err := c.Open(ctx, notePath); err != nil {
			return "", err
		}
	}
	switch avail, err := c.Check(ctx, id); {
	case err != nil:
		// The check is a courtesy; a failure to perform it must not block the
		// action itself.
	case avail == AvailMissing:
		return "", fmt.Errorf("%s is not registered — is the plugin enabled?", id)
	case avail == AvailBlocked:
		return "", fmt.Errorf("%s %s", id, BlockedHint)
	}
	return c.Command(ctx, id)
}

// FetchAll pulls every issue matching the configured JQL preset.
//
// This is the only Jira command that needs no active file, which also makes it
// the one most likely to keep working: the others read the issue key from
// whichever note is focused in the GUI.
func (c *Client) FetchAll(ctx context.Context) (string, error) {
	return c.runChecked(ctx, "", CmdJiraFetchBatch)
}

// TransitionStatus starts a workflow transition for the ticket at path.
//
// Status is never written to frontmatter directly. Doing so would rename the
// file, trip the archive automation and push the result, all while Jira still
// held the old value — the vault would look finished and Jira would disagree.
func (c *Client) TransitionStatus(ctx context.Context, notePath string) (string, error) {
	return c.runChecked(ctx, notePath, CmdJiraUpdateStatus)
}

// AddComment opens the comment composer for the ticket at path.
func (c *Client) AddComment(ctx context.Context, notePath string) (string, error) {
	return c.runChecked(ctx, notePath, CmdJiraAddComment)
}

// PushWorklogBatch sends accumulated time for the ticket at path.
//
// This command reads its input from a frontmatter property instead of
// prompting, so it is the one Jira write that needs neither a modal nor user
// interaction — the caller writes the property, then calls this.
func (c *Client) PushWorklogBatch(ctx context.Context, notePath string) (string, error) {
	return c.runChecked(ctx, notePath, CmdJiraWorklogBatch)
}
