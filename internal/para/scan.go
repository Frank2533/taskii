package para

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Folder names that carry meaning in a PARA vault.
const (
	dirTickets   = "Tickets"
	dirArchive   = "Archive"
	dirProjects  = "Projects"
	dirAreas     = "Areas"
	dirTemplates = "Templates"
	dirFileClass = "FileClasses"
)

// Note is a parsed markdown file: its frontmatter, and its body split into
// lines so edits can address a single line by number.
type Note struct {
	Path        string
	Front       map[string]any
	Lines       []string
	FrontEndIdx int // index of the closing "---"; -1 when there is no frontmatter
}

// ParseNote reads and splits a markdown note. A file with no frontmatter is
// not an error — Projects and Resources routinely have none.
func ParseNote(path string) (*Note, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Normalise CRLF so line indices match what a writer will produce.
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	lines := strings.Split(string(b), "\n")

	n := &Note{Path: path, Lines: lines, FrontEndIdx: -1, Front: map[string]any{}}
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return n, nil
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			n.FrontEndIdx = i
			break
		}
	}
	if n.FrontEndIdx < 0 {
		return n, nil
	}
	raw := strings.Join(lines[1:n.FrontEndIdx], "\n")
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		// Malformed frontmatter shouldn't hide the note's body from the
		// index; treat it as absent and carry on.
		return n, nil
	}
	if front, ok := nodeToMap(&doc); ok {
		n.Front = front
	}
	return n, nil
}

// nodeToMap converts the frontmatter document, keeping every scalar as its
// literal text.
//
// Decoding straight into map[string]any would let YAML's implicit typing claim
// "duedate: 2026-08-25" as a time.Time at midnight UTC. That silently discards
// the fact that the note wrote a bare date with no zone, and re-anchoring it
// afterwards shifts the day for any user far enough from Greenwich. Keeping the
// raw text lets parseDate anchor it in the user's configured zone instead.
func nodeToMap(doc *yaml.Node) (map[string]any, bool) {
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) == 0 {
			return nil, false
		}
		doc = doc.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		return nil, false
	}
	out := map[string]any{}
	for i := 0; i+1 < len(doc.Content); i += 2 {
		out[doc.Content[i].Value] = nodeValue(doc.Content[i+1])
	}
	return out, true
}

func nodeValue(n *yaml.Node) any {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Tag == "!!null" {
			return ""
		}
		return n.Value
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			out = append(out, nodeValue(c))
		}
		return out
	case yaml.MappingNode:
		out := map[string]any{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			out[n.Content[i].Value] = nodeValue(n.Content[i+1])
		}
		return out
	default:
		return ""
	}
}

// str coerces a frontmatter value to a display string. Frontmatter here is
// hand-edited and plugin-written, so a field can arrive as a string, a YAML
// timestamp, a list (labels, components) or null.
func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case time.Time:
		return t.Format(time.RFC3339)
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			if s := str(e); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func field(front map[string]any, key string) string { return str(front[key]) }

// dateLayouts covers what turns up in these notes: bare due dates, and Jira's
// millisecond timestamps with a colon-less offset.
var dateLayouts = []string{
	"2006-01-02",
	time.RFC3339,
	"2006-01-02T15:04:05.000-0700",
	"2006-01-02T15:04:05-0700",
	"2006-01-02 15:04:05",
}

// parseDate interprets a frontmatter date in loc. A bare date has no time or
// zone of its own, so it is anchored to midnight in the configured zone rather
// than UTC — otherwise "due today" flips a day early or late for anyone east
// or west of Greenwich.
func parseDate(v any, loc *time.Location) (time.Time, bool) {
	if loc == nil {
		loc = time.Local
	}
	if t, ok := v.(time.Time); ok {
		return t.In(loc), true
	}
	s := str(v)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range dateLayouts {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t.In(loc), true
		}
	}
	return time.Time{}, false
}

// checkboxes extracts every task line in the body, tagged with the heading it
// sits under.
func checkboxes(n *Note) []Checkbox {
	var out []Checkbox
	heading := ""
	start := n.FrontEndIdx + 1
	for i := start; i < len(n.Lines); i++ {
		line := n.Lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			heading = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			continue
		}
		done, text, ok := parseCheckbox(trimmed)
		if !ok {
			continue
		}
		out = append(out, Checkbox{Line: i, Done: done, Text: text, Heading: heading})
	}
	return out
}

// parseCheckbox matches "- [ ] text" / "* [x] text" and any marker in between,
// which Obsidian renders as a non-empty checked state.
func parseCheckbox(trimmed string) (done bool, text string, ok bool) {
	for _, bullet := range []string{"- ", "* ", "+ "} {
		if !strings.HasPrefix(trimmed, bullet) {
			continue
		}
		rest := trimmed[len(bullet):]
		if len(rest) < 3 || rest[0] != '[' || rest[2] != ']' {
			return false, "", false
		}
		marker := rest[1]
		return marker != ' ', strings.TrimSpace(rest[3:]), true
	}
	return false, "", false
}

// Index is a whole vault, parsed.
type Index struct {
	Vault     string
	Tickets   []Ticket
	Projects  []Project
	Areas     []Area
	ScannedAt time.Time
	Loc       *time.Location
}

// Scan walks the vault and builds an index. loc governs how bare dates are
// interpreted; pass the user's configured location.
func Scan(vault string, loc *time.Location) (*Index, error) {
	if loc == nil {
		loc = time.Local
	}
	info, err := os.Stat(vault)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("vault %q is not a directory", vault)
	}

	idx := &Index{Vault: vault, Loc: loc, ScannedAt: time.Now().In(loc)}
	areaSet := map[string]string{}

	err = filepath.WalkDir(vault, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable corner of the vault shouldn't abort the scan.
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path == vault {
				return nil
			}
			// Skip Obsidian's own state and the folders that hold templates
			// rather than content — a template's frontmatter has empty keys
			// that would otherwise index as a blank ticket.
			if strings.HasPrefix(name, ".") || name == dirTemplates || name == dirFileClass {
				return filepath.SkipDir
			}
			if rel, _ := filepath.Rel(vault, filepath.Dir(path)); rel == dirAreas {
				areaSet[name] = ""
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(vault, path)
		if relErr != nil {
			return nil
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) == 0 {
			return nil
		}

		switch parts[0] {
		case dirTickets:
			if t, ok := readTicket(path, loc, false); ok {
				idx.Tickets = append(idx.Tickets, t)
			}
		case dirArchive:
			if t, ok := readTicket(path, loc, true); ok {
				if t.Area == "" && len(parts) > 2 {
					// Archived notes are filed in Archive/<Area>/, so the
					// folder is a reliable fallback when the property is blank.
					t.Area = parts[1]
				}
				idx.Tickets = append(idx.Tickets, t)
			}
		case dirProjects:
			if strings.EqualFold(name, "README.md") {
				return nil
			}
			if p, ok := readProject(path, loc); ok {
				idx.Projects = append(idx.Projects, p)
			}
		case dirAreas:
			if len(parts) > 2 {
				areaSet[parts[1]] = path
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for name, notePath := range areaSet {
		idx.Areas = append(idx.Areas, Area{Name: name, Path: notePath})
	}
	idx.sort()
	return idx, nil
}

func readTicket(path string, loc *time.Location, archived bool) (Ticket, bool) {
	n, err := ParseNote(path)
	if err != nil {
		return Ticket{}, false
	}
	t := Ticket{
		Key:        field(n.Front, "key"),
		Summary:    field(n.Front, "summary"),
		Status:     field(n.Front, "status"),
		Area:       field(n.Front, "area"),
		IssueType:  field(n.Front, "issuetype"),
		Priority:   field(n.Front, "priority"),
		EpicLink:   field(n.Front, "epic_link"),
		Assignee:   field(n.Front, "assignee"),
		Sprint:     field(n.Front, "sprint"),
		Link:       field(n.Front, "link"),
		Path:       path,
		Archived:   archived,
		Checkboxes: checkboxes(n),
	}
	if t.Key == "" && t.Summary == "" {
		// Not a ticket note (a stray README, a scratch file).
		return Ticket{}, false
	}
	t.Due, t.HasDue = parseDate(n.Front["duedate"], loc)
	t.Updated, _ = parseDate(n.Front["updated"], loc)
	return t, true
}

func readProject(path string, loc *time.Location) (Project, bool) {
	n, err := ParseNote(path)
	if err != nil {
		return Project{}, false
	}
	title := field(n.Front, "title")
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	p := Project{
		Title:      title,
		Area:       field(n.Front, "area"),
		Status:     field(n.Front, "status"),
		JiraEpic:   field(n.Front, "jira_epic"),
		Path:       path,
		Checkboxes: checkboxes(n),
	}
	p.Target, p.HasTarget = parseDate(n.Front["target_date"], loc)
	return p, true
}
