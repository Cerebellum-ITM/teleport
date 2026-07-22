package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/pascualchavez/teleport/internal/theme"
)

// CommitDiffFunc renders a commit's full diff for the mirror target picker's
// viewer. Defined by the caller (cmd/) so the git + highlight I/O stays out of
// the TUI model (architecture invariant #3). width is the current render width.
type CommitDiffFunc func(sha string, width int) (ViewerContent, error)

// commitDiffMsg delivers the lazily-loaded diff back to the picker.
type commitDiffMsg struct {
	content ViewerContent
	err     error
}

// commitDiffViewer is a read-only bat-style pager that shows one commit's whole
// diff, embedded in the mirror target picker. Diff-only — there is no file⇄diff
// toggle here.
type commitDiffViewer struct {
	short   string
	subject string
	vp      viewport.Model
	width   int
	height  int
	content ViewerContent
	loading bool
	err     error
}

func newCommitDiffViewer(short, subject string, width, height int) commitDiffViewer {
	vh := height - 4 // header(1) + blank(1) + blank(1) + footer(1)
	if vh < 1 {
		vh = 1
	}
	return commitDiffViewer{
		short:   short,
		subject: subject,
		vp:      viewport.New(viewport.WithWidth(width), viewport.WithHeight(vh)),
		width:   width,
		height:  height,
		loading: true,
	}
}

func (v *commitDiffViewer) resize(width, height int) {
	v.width, v.height = width, height
	vh := height - 4
	if vh < 1 {
		vh = 1
	}
	v.vp.SetWidth(width)
	v.vp.SetHeight(vh)
}

func (v *commitDiffViewer) setContent(c ViewerContent, err error) {
	if err != nil {
		v.err = err
		v.loading = false
		return
	}
	v.content = c
	v.loading = false
	v.vp.SetContent(c.body())
	v.vp.GotoTop()
}

func (v commitDiffViewer) header() string {
	short := commitShortStyle.Render(iconCommit + v.short)
	subj := v.subject
	if chip, rest, ok := theme.TagChip(v.subject); ok {
		subj = chip + " " + rest
	}
	counts := ""
	if !v.loading && v.err == nil {
		counts = dimStyle.Render("  ·  ") +
			checkStyle.Render(fmt.Sprintf("+%d", v.content.Adds)) + " " +
			deleteStyle.Render(fmt.Sprintf("−%d", v.content.Dels))
	}
	return headerStyle.Render("  "+iconDiff) + short + dimStyle.Render("  ·  ") + subj + counts
}

func (v commitDiffViewer) footer() string {
	return dimStyle.Render("  j/k ↑/↓=scroll  ^d/^u=half-page  g/G=top/bottom  esc=back  ctrl+c=quit")
}

func (v commitDiffViewer) View() string {
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

// Update handles only scroll keys; the picker owns esc/ctrl+c.
func (v commitDiffViewer) Update(msg tea.Msg) (commitDiffViewer, tea.Cmd) {
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
