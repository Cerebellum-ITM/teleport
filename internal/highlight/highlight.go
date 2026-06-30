// Package highlight renders source files and unified diffs as ANSI text for a
// bat-style terminal pager. It wraps chroma (pure Go, no CGO) so the static
// binary invariant holds.
package highlight

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/colorprofile"

	"charm.land/lipgloss/v2"
)

// styleName is the chroma style used for code. catppuccin-mocha matches the
// dark technical workspace described in context/ui-context.md and ships with
// chroma, so no extra dependency is needed.
const styleName = "catppuccin-mocha"

// lineCap bounds how many lines we tokenise. Past it we render plain text with
// the gutter only, so a pathologically large blob can't stall the UI.
const lineCap = 5000

var (
	gutterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	// Delta-style diff gutters: a tinted two-column line-number chip per change
	// kind, sitting left of the syntax-highlighted code.
	addGutterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("83")).Background(lipgloss.Color("22"))
	delGutterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("210")).Background(lipgloss.Color("52"))
	ctxGutterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	// Full-width hunk bar: dim line range, accent section context, both on a
	// subtle bar background that the padding extends across the row.
	hunkBarStyle   = lipgloss.NewStyle().Background(lipgloss.Color("238"))
	hunkRangeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Background(lipgloss.Color("238"))
	hunkCtxStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Background(lipgloss.Color("238"))
)

// hunkRe captures the old/new starting line numbers from a unified-diff hunk
// header: "@@ -<old>[,n] +<new>[,n] @@[ section]".
var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// IsBinary reports whether b looks like a binary blob (a NUL byte in the first
// 8 KB), in which case it should not be syntax-highlighted.
func IsBinary(b []byte) bool {
	if len(b) > 8192 {
		b = b[:8192]
	}
	return bytes.IndexByte(b, 0) >= 0
}

// formatterFor maps a terminal color profile to the matching chroma formatter.
func formatterFor(p colorprofile.Profile) chroma.Formatter {
	switch p {
	case colorprofile.TrueColor:
		return formatters.TTY16m
	case colorprofile.ANSI256:
		return formatters.TTY256
	case colorprofile.ANSI:
		return formatters.TTY16
	default: // Ascii, NoTTY
		return formatters.NoOp
	}
}

// pickLexer chooses a chroma lexer by filename, falling back to content
// analysis and finally the plain lexer, and returns it coalesced plus its name.
func pickLexer(filename string, sample []byte) (chroma.Lexer, string) {
	lexer := lexers.Match(filename)
	if lexer == nil {
		lexer = lexers.Analyse(string(sample))
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	return chroma.Coalesce(lexer), lexer.Config().Name
}

// lineHighlighter returns a function that syntax-highlights a single line of
// code (no trailing newline). On any tokenise/format error it returns the line
// unchanged, so diff rendering never breaks.
func lineHighlighter(lexer chroma.Lexer, p colorprofile.Profile) func(string) string {
	style := styles.Get(styleName)
	formatter := formatterFor(p)
	return func(code string) string {
		it, err := lexer.Tokenise(nil, code)
		if err != nil {
			return code
		}
		var buf bytes.Buffer
		if formatter.Format(&buf, style, it) != nil {
			return code
		}
		return strings.TrimRight(buf.String(), "\n")
	}
}

// Code highlights src as the language inferred from filename and returns it
// with a left line-number gutter, the detected language name, and the line
// count.
func Code(src []byte, filename string, p colorprofile.Profile) (body, lang string, lines int, err error) {
	lexer, lang := pickLexer(filename, src)

	// Over the cap, skip tokenising entirely — gutter-only plain text.
	if bytes.Count(src, []byte{'\n'}) > lineCap {
		body, lines = addGutter(string(src))
		return body, lang, lines, nil
	}

	style := styles.Get(styleName)
	it, err := lexer.Tokenise(nil, string(src))
	if err != nil {
		body, lines = addGutter(string(src))
		return body, lang, lines, nil
	}
	var buf bytes.Buffer
	if err := formatterFor(p).Format(&buf, style, it); err != nil {
		body, lines = addGutter(string(src))
		return body, lang, lines, nil
	}
	body, lines = addGutter(buf.String())
	return body, lang, lines, nil
}

// Diff renders a unified-diff blob delta-style: the code on each line is
// syntax-highlighted with the lexer for filename, and a tinted two-column
// (old/new) line-number gutter marks added (green), removed (red), and context
// (dim) lines. Each hunk opens with a full-width bar (dim line range + accent
// section context) preceded by a blank separator; the redundant file-header
// block (diff --git/index/---/+++) is dropped — the viewer header already shows
// the path and SHA. width is the render width used to pad the hunk bar.
// Returns the added/removed line counts for the viewer header.
func Diff(raw []byte, filename string, width int, p colorprofile.Profile) (body string, adds, dels int) {
	lexer, _ := pickLexer(filename, raw)
	hl := lineHighlighter(lexer, p)

	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	var b strings.Builder
	var oldLn, newLn int
	inHunk := false
	firstHunk := true

	for _, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "@@"):
			if m := hunkRe.FindStringSubmatch(ln); m != nil {
				oldLn, _ = strconv.Atoi(m[1])
				newLn, _ = strconv.Atoi(m[2])
			}
			if !firstHunk {
				b.WriteByte('\n') // blank separator between hunks
			}
			firstHunk = false
			inHunk = true
			b.WriteString(hunkBar(ln, width))
		case !inHunk:
			// Drop the file-header block entirely (redundant with the viewer
			// header). Skip the line without emitting a newline.
			continue
		case strings.HasPrefix(ln, "+"):
			adds++
			b.WriteString(diffGutter(addGutterStyle, "", strconv.Itoa(newLn)) + hl(ln[1:]))
			newLn++
		case strings.HasPrefix(ln, "-"):
			dels++
			b.WriteString(diffGutter(delGutterStyle, strconv.Itoa(oldLn), "") + hl(ln[1:]))
			oldLn++
		default: // context line (leading space) or "\ No newline…"
			code := ln
			if len(ln) > 0 {
				code = ln[1:]
			}
			b.WriteString(diffGutter(ctxGutterStyle, strconv.Itoa(oldLn), strconv.Itoa(newLn)) + hl(code))
			oldLn++
			newLn++
		}
		b.WriteByte('\n')
	}
	return b.String(), adds, dels
}

// diffGutter renders the tinted two-column (old/new) line-number chip plus the
// dim separator. A blank side keeps the columns aligned across change kinds.
func diffGutter(st lipgloss.Style, old, new string) string {
	return st.Render(fmt.Sprintf(" %4s %4s ", old, new)) + gutterStyle.Render("│ ")
}

// hunkBar renders a unified-diff hunk header as a full-width bar: the
// "@@ -a,b +c,d @@" range dim and the trailing section context (the enclosing
// function/class git names) in accent, padded with the bar background across
// width columns.
func hunkBar(ln string, width int) string {
	rangePart, ctx := ln, ""
	// ln is "@@ -a,b +c,d @@[ context]"; split on the two "@@".
	if parts := strings.SplitN(ln, "@@", 3); len(parts) == 3 {
		rangePart = "@@" + parts[1] + "@@"
		ctx = strings.TrimRight(parts[2], " ")
	}
	const left = " "
	pad := width - lipgloss.Width(left+rangePart+ctx)
	if pad < 0 {
		pad = 0
	}
	return hunkBarStyle.Render(left) +
		hunkRangeStyle.Render(rangePart) +
		hunkCtxStyle.Render(ctx) +
		hunkBarStyle.Render(strings.Repeat(" ", pad))
}

// addGutter prefixes every line with a right-aligned, dim line number and a
// "│ " separator, and reports the line count.
func addGutter(s string) (string, int) {
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	width := len(fmt.Sprintf("%d", len(lines)))
	var b strings.Builder
	for i, ln := range lines {
		b.WriteString(gutterStyle.Render(fmt.Sprintf("%*d │ ", width, i+1)))
		b.WriteString(ln)
		b.WriteByte('\n')
	}
	return b.String(), len(lines)
}
