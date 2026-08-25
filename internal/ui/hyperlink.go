// Package-level helpers for rendering clickable links in the terminal.
//
// Terminal hyperlinks are OSC 8 escape sequences
// (https://gist.github.com/egmontkob/eb114294efbcd5adb1944c9f3cb5feda). A
// terminal that understands them makes the wrapped text clickable — Ctrl-click
// or a plain click, depending on the emulator — and opens the URL directly, no
// keybinding needed. A terminal that does not understand them ignores the
// bytes entirely and shows the text unchanged, so this is safe to apply
// unconditionally.
//
// x/ansi's width, wrap and truncate functions already treat OSC 8 sequences as
// zero-width (confirmed against the vendored v0.11.6, which ships hyperlink
// test fixtures through exactly those functions), so wrapping happens as the
// very last step, after a string has already been fitted and styled — the
// same "measure and style once, don't touch it again" rule the rest of this
// codebase follows for ANSI-styled spans.
package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// hyperlinkField wraps an already-rendered, already-fitted string so the whole
// span opens url when clicked. Used for structured single-link fields — a
// ticket's Jira link, its PR link — where the visible text and the
// destination are different things.
func hyperlinkField(rendered, url string) string {
	if url == "" {
		return rendered
	}
	return ansi.SetHyperlink(url) + rendered + ansi.ResetHyperlink()
}

// urlRe matches a bare http(s) URL. Trailing punctuation that is plausibly
// sentence punctuation rather than part of the URL — a period, comma, closing
// bracket — is trimmed off by linkifyURLs so "see https://x.com/y." does not
// turn the sentence's own full stop into part of the link.
var urlRe = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `]+`)

// linkifyURLs finds bare URLs inside an already-rendered string and makes each
// one clickable in place.
//
// This operates on the final rendered output, not on plain text before
// styling, which only works because every call site here renders a whole line
// with a single style.Render(...) call — the content bytes are untouched by
// that, just wrapped in one pair of SGR codes — so a URL substring found in
// the plain text is still present verbatim in the rendered string and can be
// matched and wrapped directly. If a call site ever composes a line from
// multiple separately-rendered spans, this assumption breaks and linkifying
// needs to move earlier, before those spans are joined.
func linkifyURLs(rendered string) string {
	return urlRe.ReplaceAllStringFunc(rendered, func(url string) string {
		trimmed := strings.TrimRight(url, ".,;:!?)]}")
		if trimmed == "" {
			return url
		}
		suffix := url[len(trimmed):]
		return hyperlinkField(trimmed, trimmed) + suffix
	})
}
