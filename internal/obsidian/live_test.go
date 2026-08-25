package obsidian

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveCheck exercises Check against a running Obsidian. It is skipped
// unless TASKII_LIVE_OBSIDIAN is set, because it needs the desktop app running
// and the CLI enabled — and because the CLI launches the app when it is not,
// which a test suite must never do on its own.
func TestLiveCheck(t *testing.T) {
	if os.Getenv("TASKII_LIVE_OBSIDIAN") == "" {
		t.Skip("set TASKII_LIVE_OBSIDIAN=1 with Obsidian running to exercise this")
	}
	c := New(os.Getenv("TASKII_TEST_VAULT"))
	if !c.Available() {
		t.Fatal(c.Unavailable())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, id := range []string{
		CmdJiraFetchBatch,
		CmdJiraUpdateStatus,
		CmdJiraAddComment,
		CmdJiraRebuildCache,
	} {
		avail, err := c.Check(ctx, id)
		if err != nil {
			t.Fatalf("Check(%s): %v", id, err)
		}
		t.Logf("%-45s %v", id, avail)
		if avail == AvailMissing {
			t.Errorf("%s is not registered", id)
		}
	}

	// A command that does not exist must be reported as missing rather than
	// as merely unavailable, so the UI can tell the two apart.
	avail, err := c.Check(ctx, "jira-sync:no-such-command")
	if err != nil {
		t.Fatal(err)
	}
	if avail != AvailMissing {
		t.Errorf("unknown command reported %v, want AvailMissing", avail)
	}
}
