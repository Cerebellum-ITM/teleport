// Package theme is the single source of truth for teleport's terminal colors.
// Nothing outside this package — except internal/highlight, which is chroma's
// domain — may write lipgloss.Color("NN"); every cmd and TUI style references
// a token defined here so the palette stays coherent.
package theme

import (
	"image/color"
	"regexp"

	lipgloss "charm.land/lipgloss/v2"
)

// Semantic color tokens.
var (
	Accent     = lipgloss.Color("212") // cursor / selection (pink)
	Success    = lipgloss.Color("82")  // ✓, sent, OK (green)
	Warn       = lipgloss.Color("214") // unselected toggle, warnings (amber)
	Danger     = lipgloss.Color("203") // delete, fail, error, missing (red)
	HeaderBg   = lipgloss.Color("62")  // title-bar background / header text
	HeaderFg   = lipgloss.Color("230") // title-bar foreground
	Section    = lipgloss.Color("104") // section headers (help)
	Icon       = lipgloss.Color("116") // file-type / section glyphs, bars (cyan)
	Path       = lipgloss.Color("86")  // remote paths
	Gold       = lipgloss.Color("220") // gold accents (examples, ship active)
	TextBright = lipgloss.Color("255") // highlighted names / values
	Text       = lipgloss.Color("252") // primary body text, progress stats
	TextDim    = lipgloss.Color("245") // secondary text
	TextFaint  = lipgloss.Color("241") // tertiary text, gutters, separators
	Hint       = lipgloss.Color("150") // guide / help lines (soft green)
)

// CommitPalette assigns a distinct accent to each commit so a multi-commit list
// (the beam file picker) reads at a glance. Assigned in display order and reused
// (mod length) past the end. Moved verbatim from beam — values unchanged.
var CommitPalette = []lipgloss.Style{
	lipgloss.NewStyle().Foreground(lipgloss.Color("39")),  // blue
	lipgloss.NewStyle().Foreground(lipgloss.Color("45")),  // cyan
	lipgloss.NewStyle().Foreground(lipgloss.Color("43")),  // teal
	lipgloss.NewStyle().Foreground(lipgloss.Color("81")),  // sky
	lipgloss.NewStyle().Foreground(lipgloss.Color("220")), // gold
	lipgloss.NewStyle().Foreground(lipgloss.Color("215")), // light orange
	lipgloss.NewStyle().Foreground(lipgloss.Color("208")), // orange
	lipgloss.NewStyle().Foreground(lipgloss.Color("209")), // salmon
	lipgloss.NewStyle().Foreground(lipgloss.Color("205")), // pink
	lipgloss.NewStyle().Foreground(lipgloss.Color("213")), // light magenta
	lipgloss.NewStyle().Foreground(lipgloss.Color("199")), // deep pink
	lipgloss.NewStyle().Foreground(lipgloss.Color("171")), // magenta
	lipgloss.NewStyle().Foreground(lipgloss.Color("141")), // purple
	lipgloss.NewStyle().Foreground(lipgloss.Color("99")),  // violet
	lipgloss.NewStyle().Foreground(lipgloss.Color("147")), // periwinkle
	lipgloss.NewStyle().Foreground(lipgloss.Color("105")), // indigo
}

// tagColors maps a CommitCraft-style [TAG] prefix to a semantic color.
var tagColors = map[string]color.Color{
	"ADD":   lipgloss.Color("41"),  // green
	"FIX":   lipgloss.Color("209"), // red
	"IMP":   lipgloss.Color("141"), // purple
	"REF":   lipgloss.Color("141"), // purple
	"DOC":   lipgloss.Color("39"),  // blue
	"MERGE": lipgloss.Color("45"),  // cyan
	"REL":   lipgloss.Color("220"), // gold
	"DEL":   lipgloss.Color("203"), // red
	"REM":   lipgloss.Color("203"), // red
}

var tagRe = regexp.MustCompile(`^\[([A-Z]+)\]\s*`)

// TagChip splits a commit subject into a colored [TAG] chip and the remaining
// text. ok is false when the subject has no leading [TAG]; then chip is "" and
// rest is the whole subject. The chip is foreground-only (bold) — no background
// — so it never clashes with the selected-row highlight.
func TagChip(subject string) (chip, rest string, ok bool) {
	m := tagRe.FindStringSubmatch(subject)
	if m == nil {
		return "", subject, false
	}
	col, known := tagColors[m[1]]
	if !known {
		col = TextDim
	}
	chip = lipgloss.NewStyle().Bold(true).Foreground(col).Render("[" + m[1] + "]")
	return chip, subject[len(m[0]):], true
}
