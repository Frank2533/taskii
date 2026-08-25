package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHyperlinkFieldWrapsWholeSpan(t *testing.T) {
	got := hyperlinkField("Jira", "https://data-impact.atlassian.net/browse/SCRAP-1")
	want := ansi.SetHyperlink("https://data-impact.atlassian.net/browse/SCRAP-1") + "Jira" + ansi.ResetHyperlink()
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !strings.Contains(got, "Jira") {
		t.Error("the visible label was lost")
	}
}

func TestHyperlinkFieldNoopsOnEmptyURL(t *testing.T) {
	if got := hyperlinkField("plain text", ""); got != "plain text" {
		t.Errorf("got %q, want the text unchanged", got)
	}
}

func TestLinkifyURLsWrapsABareURL(t *testing.T) {
	got := linkifyURLs("see https://example.com/x for details")
	if !strings.Contains(got, ansi.SetHyperlink("https://example.com/x")) {
		t.Errorf("URL was not wrapped: %q", got)
	}
	if !strings.Contains(got, "https://example.com/x") {
		t.Error("the visible URL text was lost")
	}
	if !strings.Contains(got, "see ") || !strings.Contains(got, " for details") {
		t.Errorf("surrounding text was disturbed: %q", got)
	}
}

// A sentence's own punctuation must not become part of the link target.
func TestLinkifyURLsTrimsTrailingPunctuation(t *testing.T) {
	got := linkifyURLs("check https://example.com/x.")
	if !strings.Contains(got, ansi.SetHyperlink("https://example.com/x")) {
		t.Errorf("trailing period was not trimmed from the target: %q", got)
	}
	if strings.Contains(got, ansi.SetHyperlink("https://example.com/x.")) {
		t.Errorf("the period leaked into the link target: %q", got)
	}
	if !strings.HasSuffix(strings.TrimSuffix(got, ansi.ResetHyperlink()), ".") {
		// The period itself must still be visible, just outside the link.
		t.Errorf("the trailing period is missing from the visible text: %q", got)
	}
}

func TestLinkifyURLsHandlesMultipleLinks(t *testing.T) {
	got := linkifyURLs("https://a.com and https://b.com")
	for _, u := range []string{"https://a.com", "https://b.com"} {
		if !strings.Contains(got, ansi.SetHyperlink(u)) {
			t.Errorf("%s was not wrapped: %q", u, got)
		}
	}
}

func TestLinkifyURLsLeavesPlainTextAlone(t *testing.T) {
	if got := linkifyURLs("no links here"); got != "no links here" {
		t.Errorf("plain text was changed: %q", got)
	}
}

// A styled line — the shape every real call site produces — must survive
// linkifying: the SGR wrapper is untouched and the URL substring inside it is
// still found and wrapped.
func TestLinkifyURLsWorksInsideAStyledLine(t *testing.T) {
	styled := taskStyle.Render("note: https://example.com/y more text")
	got := linkifyURLs(styled)
	if !strings.Contains(got, ansi.SetHyperlink("https://example.com/y")) {
		t.Errorf("URL inside styled text was not wrapped: %q", got)
	}
	if !strings.Contains(got, "note:") || !strings.Contains(got, "more text") {
		t.Errorf("styled text content was lost: %q", got)
	}
}
