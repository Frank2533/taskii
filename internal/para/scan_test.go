package para

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeNote is a fixture helper: fixtures are used rather than a real vault so
// the tests never depend on someone's private notes.
func writeNote(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixtureVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	writeNote(t, root, "Tickets/AAA-1 In Progress live one.md", `---
fileClass: JiraIssue
area: QCOM
key: AAA-1
summary: live one
status: In Progress
issuetype: Task
epic_link: AAA-100
duedate: 2026-08-25
---

## Notes & Sub-tasks
- [x] first step
- [ ] second step

## Work Log / Updates
- 
`)

	// Closed, but no area: the archive automation needs both, so this note is
	// stranded in Tickets/ and nothing else surfaces it.
	writeNote(t, root, "Tickets/AAA-2 Done stranded.md", `---
key: AAA-2
summary: stranded
status: Done
area: 
---
`)

	writeNote(t, root, "Archive/QCOM/AAA-3 Done archived.md", `---
key: AAA-3
summary: archived
status: Done
area: QCOM
---
`)

	// Archived with a blank property: the folder should supply the area.
	writeNote(t, root, "Archive/Client Requests/AAA-4 Done foldered.md", `---
key: AAA-4
summary: foldered
status: Done
---
`)

	writeNote(t, root, "Projects/Normalize cities.md", `---
status: active
area: QCOM
jira_epic: AAA-100
target_date: 2026-09-01
---

# Normalize cities
`)

	writeNote(t, root, "Projects/README.md", "# not a project\n")
	writeNote(t, root, "Areas/QCOM/Info.md", "---\nstatus: active\n---\n\n# QCOM\n")
	writeNote(t, root, "Areas/Client Requests/Info.md", "---\nstatus: active\n---\n")

	// Templates carry empty keys that would index as blank ticket notes.
	writeNote(t, root, "Templates/Jira.md", "---\nkey: \nsummary: \nstatus: \n---\n")
	writeNote(t, root, "FileClasses/JiraIssue.md", "---\nfields: []\n---\n")
	writeNote(t, root, ".obsidian/plugins/x/data.json", "{}")

	return root
}

func TestScanClassifiesNotes(t *testing.T) {
	idx, err := Scan(fixtureVault(t), time.UTC)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(idx.Tickets), 4; got != want {
		t.Errorf("tickets = %d, want %d", got, want)
	}
	if got, want := len(idx.Projects), 1; got != want {
		t.Errorf("projects = %d, want %d (README.md must not count)", got, want)
	}
	if got, want := len(idx.Areas), 2; got != want {
		t.Errorf("areas = %d, want %d", got, want)
	}
	if got, want := len(idx.Active()), 1; got != want {
		t.Errorf("active = %d, want %d", got, want)
	}
}

func TestScanSkipsTemplates(t *testing.T) {
	idx, err := Scan(fixtureVault(t), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range idx.Tickets {
		if tk.Key == "" {
			t.Fatalf("indexed a note with no key from %s — a template leaked in", tk.Path)
		}
	}
}

func TestUnfiledFindsStrandedTicketsOnly(t *testing.T) {
	idx, err := Scan(fixtureVault(t), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	unfiled := idx.Unfiled()
	if len(unfiled) != 1 {
		t.Fatalf("unfiled = %d, want 1: %+v", len(unfiled), unfiled)
	}
	if unfiled[0].Key != "AAA-2" {
		t.Errorf("unfiled key = %q, want AAA-2", unfiled[0].Key)
	}
}

func TestArchiveFolderSuppliesMissingArea(t *testing.T) {
	idx, err := Scan(fixtureVault(t), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	tk, ok := idx.Ticket("AAA-4")
	if !ok {
		t.Fatal("AAA-4 not indexed")
	}
	if tk.Area != "Client Requests" {
		t.Errorf("area = %q, want %q from the folder name", tk.Area, "Client Requests")
	}
}

func TestCheckboxesCarryLineAndHeading(t *testing.T) {
	idx, err := Scan(fixtureVault(t), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	tk, ok := idx.Ticket("AAA-1")
	if !ok {
		t.Fatal("AAA-1 not indexed")
	}
	if len(tk.Checkboxes) != 2 {
		t.Fatalf("checkboxes = %d, want 2", len(tk.Checkboxes))
	}
	if !tk.Checkboxes[0].Done || tk.Checkboxes[1].Done {
		t.Errorf("done states = %v/%v, want true/false", tk.Checkboxes[0].Done, tk.Checkboxes[1].Done)
	}
	for _, c := range tk.Checkboxes {
		if c.Heading != "Notes & Sub-tasks" {
			t.Errorf("heading = %q, want %q", c.Heading, "Notes & Sub-tasks")
		}
	}
	// The line index must address the real file line, since toggling rewrites
	// exactly that line.
	body, err := os.ReadFile(tk.Path)
	if err != nil {
		t.Fatal(err)
	}
	lines := splitLines(string(body))
	if got := lines[tk.Checkboxes[1].Line]; got != "- [ ] second step" {
		t.Errorf("line %d = %q, want the second checkbox", tk.Checkboxes[1].Line, got)
	}
}

func TestForProjectJoinsOnEpic(t *testing.T) {
	idx, err := Scan(fixtureVault(t), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Projects) != 1 {
		t.Fatalf("projects = %d", len(idx.Projects))
	}
	got := idx.ForProject(idx.Projects[0])
	if len(got) != 1 || got[0].Key != "AAA-1" {
		t.Errorf("ForProject = %+v, want just AAA-1", got)
	}
}

// A bare due date has no zone. Anchoring it to UTC would shift the day for
// anyone far enough east or west, so it must land at midnight in the zone the
// user configured.
func TestBareDateAnchorsToConfiguredZone(t *testing.T) {
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	idx, err := Scan(fixtureVault(t), kolkata)
	if err != nil {
		t.Fatal(err)
	}
	tk, _ := idx.Ticket("AAA-1")
	if !tk.HasDue {
		t.Fatal("AAA-1 has no due date")
	}
	y, m, d := tk.Due.Date()
	if y != 2026 || m != time.August || d != 25 {
		t.Errorf("due = %v, want 2026-08-25 local", tk.Due)
	}
	if h := tk.Due.Hour(); h != 0 {
		t.Errorf("due hour = %d, want midnight in %s", h, kolkata)
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// TestRealVault is a smoke check against an actual vault. It is skipped unless
// TASKII_TEST_VAULT names one, so the suite stays hermetic by default.
func TestRealVault(t *testing.T) {
	root := os.Getenv("TASKII_TEST_VAULT")
	if root == "" {
		t.Skip("set TASKII_TEST_VAULT to a vault path to run")
	}
	idx, err := Scan(root, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("tickets=%d projects=%d areas=%v", len(idx.Tickets), len(idx.Projects), idx.AreaNames())
	t.Logf("active=%d unfiled=%d events=%d", len(idx.Active()), len(idx.Unfiled()), len(idx.Events()))
	for _, tk := range idx.Active() {
		t.Logf("  %-12s %-14s area=%-18q epic=%-10s checkboxes=%d", tk.Status, tk.Key, tk.Area, tk.EpicLink, len(tk.Checkboxes))
	}
	if len(idx.Tickets) == 0 {
		t.Error("indexed no tickets at all")
	}
}
