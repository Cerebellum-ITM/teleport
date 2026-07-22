package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/pascualchavez/teleport/internal/git"
	"github.com/pascualchavez/teleport/internal/theme"
)

var mirrorGuideStyle = lipgloss.NewStyle().Foreground(theme.Hint)

type mirrorTargetModel struct {
	commits  []git.Commit
	base     string
	cursor   int
	width    int
	height   int
	chosen   git.Commit
	ok       bool
	quitting bool

	load    CommitDiffFunc   // caller-supplied diff loader (does the git+highlight I/O)
	viewing bool             // true while the commit-diff pager is open
	viewer  commitDiffViewer // the pager, valid only while viewing
}

// RunMirrorTargetPicker shows the commits ahead of the remote (newest first)
// and returns the commit chosen as the new remote tip. The reflected range is
// always the contiguous span from the remote HEAD up to (and including) the
// chosen commit, so every commit keeps its exact hash. `load` renders a
// commit's diff for the `d` preview. The bool is false when the user cancels.
func RunMirrorTargetPicker(commits []git.Commit, remoteHEAD string, load CommitDiffFunc) (git.Commit, bool, error) {
	m := mirrorTargetModel{
		commits: commits,
		base:    shortHash(remoteHEAD),
		width:   80,
		height:  24,
		load:    load,
	}
	p := tea.NewProgram(m)
	fm, err := p.Run()
	if err != nil {
		return git.Commit{}, false, err
	}
	f := fm.(mirrorTargetModel)
	return f.chosen, f.ok, nil
}

func (m mirrorTargetModel) Init() tea.Cmd { return nil }

// loadCmd fetches and renders the diff for one commit off the UI thread.
func (m mirrorTargetModel) loadCmd(sha string) tea.Cmd {
	load, width := m.load, m.width
	return func() tea.Msg {
		c, err := load(sha, width)
		return commitDiffMsg{content: c, err: err}
	}
}

func (m mirrorTargetModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.viewing {
			m.viewer.resize(m.width, m.height)
		}
		return m, nil
	case commitDiffMsg:
		m.viewer.setContent(msg.content, msg.err)
		return m, nil
	case tea.KeyPressMsg:
		if m.viewing {
			return m.updateViewing(msg)
		}
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.quitting = true
			return m, tea.Quit
		case "up", "k", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "j", "ctrl+n":
			if m.cursor < len(m.commits)-1 {
				m.cursor++
			}
			return m, nil
		case "d":
			if m.load != nil && len(m.commits) > 0 {
				c := m.commits[m.cursor]
				m.viewer = newCommitDiffViewer(c.Short, c.Subject, m.width, m.height)
				m.viewing = true
				return m, m.loadCmd(c.SHA)
			}
			return m, nil
		case "enter":
			if len(m.commits) > 0 {
				m.chosen = m.commits[m.cursor]
				m.ok = true
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

// updateViewing handles keys while the diff pager is open. esc/q close it (back
// to the picker, cursor intact); ctrl+c quits; everything else scrolls.
func (m mirrorTargetModel) updateViewing(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "q":
		m.viewing = false
		return m, nil
	}
	var cmd tea.Cmd
	m.viewer, cmd = m.viewer.Update(msg)
	return m, cmd
}

// shortHash renders a base SHA for display: "∅" for an unborn/empty remote.
func shortHash(s string) string {
	if s == "" {
		return "∅"
	}
	if len(s) >= 7 {
		return s[:7]
	}
	return s
}

func (m mirrorTargetModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	if m.viewing {
		return tea.NewView(m.viewer.View())
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("  Advance remote branch to…") + "\n\n")

	// chrome: header(1) + blank(1) + base(1) + blank(1) + guide(1) + footer(1) = 6
	win := computeWindow(len(m.commits), m.cursor, m.height-6)
	if h := scrollUpHint(win.above); h != "" {
		b.WriteString(h + "\n")
	}
	for i := win.start; i < win.end; i++ {
		c := m.commits[i]
		prefix := "    "
		if i == m.cursor {
			prefix = "  " + cursorStyle.Render("▶") + " "
		}
		subject := c.Subject
		if chip, rest, ok := theme.TagChip(c.Subject); ok {
			subject = chip + " " + rest
		}
		line := prefix + commitShortStyle.Render(c.Short) + "  " + subject +
			"  " + dimStyle.Render(c.RelDate)
		b.WriteString(line + "\n")
	}
	if h := scrollDownHint(win.below); h != "" {
		b.WriteString(h + "\n")
	}

	b.WriteString(dimStyle.Render("  ── "+m.base+" (remote HEAD)") + "\n\n")

	sel := m.base
	if len(m.commits) > 0 {
		sel = m.commits[m.cursor].Short
	}
	b.WriteString(mirrorGuideStyle.Render(
		fmt.Sprintf("  reflects %s..%s — contiguous, same hash", m.base, sel)) + "\n")
	b.WriteString(dimStyle.Render("  ↑↓=navigate  d=diff  enter=confirm  esc/q=cancel") + "\n")
	return tea.NewView(b.String())
}
