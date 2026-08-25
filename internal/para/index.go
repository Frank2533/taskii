package para

import (
	"sort"
	"strings"
	"time"
)

func (idx *Index) sort() {
	sort.Slice(idx.Tickets, func(i, j int) bool {
		return idx.Tickets[i].Key < idx.Tickets[j].Key
	})
	sort.Slice(idx.Projects, func(i, j int) bool {
		return strings.ToLower(idx.Projects[i].Title) < strings.ToLower(idx.Projects[j].Title)
	})
	sort.Slice(idx.Areas, func(i, j int) bool {
		return idx.Areas[i].Name < idx.Areas[j].Name
	})
	sort.Slice(idx.LocalTasks, func(i, j int) bool {
		if idx.LocalTasks[i].Done != idx.LocalTasks[j].Done {
			return !idx.LocalTasks[i].Done
		}
		return strings.ToLower(idx.LocalTasks[i].Title) < strings.ToLower(idx.LocalTasks[j].Title)
	})
}

// OpenLocalTasks returns unfinished local tasks.
func (idx *Index) OpenLocalTasks() []LocalTask {
	var out []LocalTask
	for _, t := range idx.LocalTasks {
		if !t.Done && !t.Archived {
			out = append(out, t)
		}
	}
	return out
}

// Ticket looks a ticket up by Jira key.
//
// Callers must resolve by key at the moment they act, never by a remembered
// path: the vault renames ticket files from their properties on a short
// debounce, so a path captured a second ago can already be wrong.
func (idx *Index) Ticket(key string) (Ticket, bool) {
	for _, t := range idx.Tickets {
		if strings.EqualFold(t.Key, key) {
			return t, true
		}
	}
	return Ticket{}, false
}

// Active returns tickets that are not closed, in key order.
func (idx *Index) Active() []Ticket {
	var out []Ticket
	for _, t := range idx.Tickets {
		if !t.Closed() && !t.Archived {
			out = append(out, t)
		}
	}
	return out
}

// Unfiled returns closed tickets that never got an Area, which the vault's
// archive automation therefore left behind in Tickets/.
func (idx *Index) Unfiled() []Ticket {
	var out []Ticket
	for _, t := range idx.Tickets {
		if t.Stranded() {
			out = append(out, t)
		}
	}
	return out
}

// InArea returns every ticket filed under an area, archived or not.
func (idx *Index) InArea(area string) []Ticket {
	var out []Ticket
	for _, t := range idx.Tickets {
		if strings.EqualFold(strings.TrimSpace(t.Area), strings.TrimSpace(area)) {
			out = append(out, t)
		}
	}
	return out
}

// ForProject resolves the epic join that the vault's Dataview queries perform:
// a project names an epic key, and tickets point back at it through epic_link.
func (idx *Index) ForProject(p Project) []Ticket {
	if strings.TrimSpace(p.JiraEpic) == "" {
		return nil
	}
	var out []Ticket
	for _, t := range idx.Tickets {
		if strings.EqualFold(strings.TrimSpace(t.EpicLink), strings.TrimSpace(p.JiraEpic)) {
			out = append(out, t)
		}
	}
	return out
}

// AreaNames lists areas from the Areas/ folders, plus any area value that only
// appears on a ticket — a vault can reference an area whose folder was never
// created, and hiding those would hide the tickets filed under them.
func (idx *Index) AreaNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range idx.Areas {
		if !seen[a.Name] {
			seen[a.Name] = true
			out = append(out, a.Name)
		}
	}
	for _, t := range idx.Tickets {
		a := strings.TrimSpace(t.Area)
		if a != "" && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	sort.Strings(out)
	return out
}

// Event is a dated thing to show on a calendar or agenda.
type Event struct {
	Date   time.Time
	AllDay bool
	Title  string
	Key    string // Jira key when the source is a ticket
	Kind   string // "ticket" | "project"
	Path   string
	Status string
}

// Events collects everything in the vault that carries a date.
func (idx *Index) Events() []Event {
	var out []Event
	for _, t := range idx.Tickets {
		if !t.HasDue || t.Closed() {
			continue
		}
		out = append(out, Event{
			Date: t.Due, AllDay: true, Title: t.Title(), Key: t.Key,
			Kind: "ticket", Path: t.Path, Status: t.Status,
		})
	}
	for _, p := range idx.Projects {
		if !p.HasTarget {
			continue
		}
		out = append(out, Event{
			Date: p.Target, AllDay: true, Title: p.Title,
			Kind: "project", Path: p.Path, Status: p.Status,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date.Equal(out[j].Date) {
			return out[i].Title < out[j].Title
		}
		return out[i].Date.Before(out[j].Date)
	})
	return out
}

// Match is one fuzzy-search hit.
type Match struct {
	Label string
	Path  string
	Key   string
	Kind  string
	score int
}

// Search does a subsequence match over tickets and projects, ranking exact
// substring hits above scattered ones. Good enough for a few hundred notes and
// fast enough to rerun on every keystroke.
func (idx *Index) Search(query string) []Match {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var out []Match
	consider := func(label, path, key, kind string) {
		s, ok := score(strings.ToLower(label), q)
		if !ok {
			return
		}
		out = append(out, Match{Label: label, Path: path, Key: key, Kind: kind, score: s})
	}
	for _, t := range idx.Tickets {
		consider(t.Title(), t.Path, t.Key, "ticket")
	}
	for _, p := range idx.Projects {
		consider(p.Title, p.Path, "", "project")
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	return out
}

// score rewards a contiguous substring, then falls back to subsequence order.
// Higher is better; ok is false when the query does not match at all.
func score(hay, needle string) (int, bool) {
	if i := strings.Index(hay, needle); i >= 0 {
		// Earlier matches rank higher, and a shorter haystack means the query
		// covers more of it.
		return 1000 - i - len(hay)/10, true
	}
	hi := 0
	points := 0
	for _, r := range needle {
		found := strings.IndexRune(hay[hi:], r)
		if found < 0 {
			return 0, false
		}
		hi += found + 1
		points++
	}
	return points, true
}
