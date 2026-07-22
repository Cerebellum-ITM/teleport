package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/pascualchavez/teleport/internal/git"
)

var mirrorGuideStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("150"))

type mirrorTargetModel struct {
	commits  []git.Commit
	base     string
	cursor   int
	height   int
	chosen   git.Commit
	ok       bool
	quitting bool
}

// RunMirrorTargetPicker shows the commits ahead of the remote (newest first)
// and returns the commit chosen as the new remote tip. The reflected range is
// always the contiguous span from the remote HEAD up to (and including) the
// chosen commit, so every commit keeps its exact hash. The bool is false when
// the user cancels.
func RunMirrorTargetPicker(commits []git.Commit, remoteHEAD string) (git.Commit, bool, error) {
	m := mirrorTargetModel{
		commits: commits,
		base:    shortHash(remoteHEAD),
		height:  24,
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

func (m mirrorTargetModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		return m, nil
	case tea.KeyPressMsg:
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
			prefix = "  ▶ "
		}
		line := prefix + boldStyle.Render(c.Short) + "  " + c.Subject +
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
	b.WriteString(dimStyle.Render("  ↑↓=navigate  enter=confirm  esc/q=cancel") + "\n")
	return tea.NewView(b.String())
}
