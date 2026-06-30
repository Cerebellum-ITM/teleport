package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/pascualchavez/teleport/internal/git"
)

const iconDiff = "󰦓 "

// ViewerMode selects what the beam file viewer shows.
type ViewerMode int

const (
	ViewFile ViewerMode = iota // full blob, syntax highlighted
	ViewDiff                   // unified diff the commit introduced
)

// ViewerContent is the rendered, ready-to-scroll payload the caller produces
// for one (file, mode) pair. The caller owns all git + highlight I/O; the
// viewer only displays the result (architecture invariant #3).
type ViewerContent struct {
	Body   string // ANSI text, already highlighted and gutter-prefixed
	Lang   string // language name for the file header
	Lines  int    // line count for the file header
	Adds   int    // added lines, for the diff header
	Dels   int    // removed lines, for the diff header
	Binary bool   // blob is binary — show a placeholder instead of Body
	Bytes  int    // size for the binary placeholder
	Note   string // optional header note, e.g. "(before delete)"
}

// FileContentFunc loads and renders a file's contents or diff. Defined by the
// caller (cmd/) so the I/O stays out of the TUI model. width is the current
// render width, used to size full-width diff decorations.
type FileContentFunc func(fc git.FileChange, mode ViewerMode, width int) (ViewerContent, error)

// viewerContentMsg delivers a lazily-loaded ViewerContent back to the picker.
type viewerContentMsg struct {
	mode    ViewerMode
	content ViewerContent
	err     error
}

// beamFileViewer is a read-only bat-style pager embedded in the beam file
// picker. It scrolls one file (or its diff) and caches both modes so tab and
// re-open are instant.
type beamFileViewer struct {
	fc       git.FileChange
	short    string
	shaStyle lipgloss.Style
	mode     ViewerMode
	vp       viewport.Model
	width    int
	height   int

	idx   int // 1-based position of fc within the active file range
	total int // files in the active range (the set ←/→ steps through)

	cache   map[ViewerMode]ViewerContent
	loaded  map[ViewerMode]bool
	loading bool
	err     error
}

func newBeamFileViewer(fc git.FileChange, short string, shaStyle lipgloss.Style, mode ViewerMode, idx, total, width, height int) beamFileViewer {
	vh := height - 4 // header(1) + blank(1) + blank(1) + footer(1)
	if vh < 1 {
		vh = 1
	}
	return beamFileViewer{
		fc:       fc,
		short:    short,
		shaStyle: shaStyle,
		mode:     mode,
		idx:      idx,
		total:    total,
		vp:       viewport.New(viewport.WithWidth(width), viewport.WithHeight(vh)),
		width:    width,
		height:   height,
		cache:    map[ViewerMode]ViewerContent{},
		loaded:   map[ViewerMode]bool{},
		loading:  true,
	}
}

func (v *beamFileViewer) resize(width, height int) {
	v.width, v.height = width, height
	vh := height - 4
	if vh < 1 {
		vh = 1
	}
	v.vp.SetWidth(width)
	v.vp.SetHeight(vh)
}

// setContent records a loaded payload and, if it is the mode on screen,
// pushes it into the viewport.
func (v *beamFileViewer) setContent(mode ViewerMode, c ViewerContent, err error) {
	if err != nil {
		if mode == v.mode {
			v.err = err
			v.loading = false
		}
		return
	}
	v.cache[mode] = c
	v.loaded[mode] = true
	if mode == v.mode {
		v.show(mode)
	}
}

// show points the viewport at a cached mode.
func (v *beamFileViewer) show(mode ViewerMode) {
	v.err = nil
	v.loading = false
	v.vp.SetContent(v.cache[mode].body())
	v.vp.GotoTop()
}

func (c ViewerContent) body() string {
	if c.Binary {
		return dimStyle.Render(fmt.Sprintf("  「binary file · %d bytes」", c.Bytes))
	}
	if strings.TrimSpace(c.Body) == "" {
		return dimStyle.Render("  「empty」")
	}
	return c.Body
}

func (v beamFileViewer) header() string {
	short := v.shaStyle.Render(iconCommit + v.short)
	note := ""
	if c, ok := v.cache[v.mode]; ok && c.Note != "" {
		note = dimStyle.Render("  " + c.Note)
	}
	if v.mode == ViewDiff {
		c := v.cache[ViewDiff]
		counts := checkStyle.Render(fmt.Sprintf("+%d", c.Adds)) + " " + deleteStyle.Render(fmt.Sprintf("−%d", c.Dels))
		return headerStyle.Render("  "+iconDiff+v.fc.Path) +
			dimStyle.Render("  ·  diff  ·  ") + short + dimStyle.Render("  ·  ") + counts + note
	}
	c := v.cache[ViewFile]
	meta := dimStyle.Render(fmt.Sprintf("  ·  %s  ·  ", c.Lang)) + short + dimStyle.Render(fmt.Sprintf("  ·  %d lines", c.Lines))
	return headerStyle.Render("  "+fileTypeIcon(v.fc.Path)+v.fc.Path) + meta + note
}

func (v beamFileViewer) footer() string {
	swap := "tab=diff"
	if v.mode == ViewDiff {
		swap = "tab=file"
	}
	nav := ""
	if v.total > 1 {
		nav = fmt.Sprintf("  ←/→=file %d/%d", v.idx, v.total)
	}
	return dimStyle.Render("  j/k ↑/↓=scroll  ^d/^u=half-page  g/G=top/bottom  " + swap + nav + "  esc=back  ctrl+c=quit")
}

func (v beamFileViewer) View() string {
	var b strings.Builder
	b.WriteString(v.header() + "\n\n")
	switch {
	case v.err != nil:
		b.WriteString(deleteStyle.Render("  " + v.err.Error()))
	case v.loading:
		b.WriteString(dimStyle.Render("  loading…"))
	default:
		b.WriteString(v.vp.View())
	}
	b.WriteString("\n\n" + v.footer())
	return b.String()
}

// Update handles only scroll keys; the picker owns esc/tab/ctrl+c.
func (v beamFileViewer) Update(msg tea.Msg) (beamFileViewer, tea.Cmd) {
	if km, ok := msg.(tea.KeyPressMsg); ok {
		switch km.String() {
		case "g":
			v.vp.GotoTop()
			return v, nil
		case "G":
			v.vp.GotoBottom()
			return v, nil
		}
	}
	var cmd tea.Cmd
	v.vp, cmd = v.vp.Update(msg)
	return v, cmd
}
