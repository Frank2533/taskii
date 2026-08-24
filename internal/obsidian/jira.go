package obsidian

import "context"

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

// FetchAll pulls every issue matching the configured JQL preset.
//
// This is the only Jira command that needs no active file, which also makes it
// the one most likely to keep working: the others read the issue key from
// whichever note is focused in the GUI.
func (c *Client) FetchAll(ctx context.Context) (string, error) {
	return c.Command(ctx, CmdJiraFetchBatch)
}

// TransitionStatus starts a workflow transition for the ticket at path.
//
// Status is never written to frontmatter directly. Doing so would rename the
// file, trip the archive automation and push the result, all while Jira still
// held the old value — the vault would look finished and Jira would disagree.
func (c *Client) TransitionStatus(ctx context.Context, notePath string) (string, error) {
	return c.OpenThenCommand(ctx, notePath, CmdJiraUpdateStatus)
}

// AddComment opens the comment composer for the ticket at path.
func (c *Client) AddComment(ctx context.Context, notePath string) (string, error) {
	return c.OpenThenCommand(ctx, notePath, CmdJiraAddComment)
}

// PushWorklogBatch sends accumulated time for the ticket at path.
//
// This command reads its input from a frontmatter property instead of
// prompting, so it is the one Jira write that needs neither a modal nor user
// interaction — the caller writes the property, then calls this.
func (c *Client) PushWorklogBatch(ctx context.Context, notePath string) (string, error) {
	return c.OpenThenCommand(ctx, notePath, CmdJiraWorklogBatch)
}
