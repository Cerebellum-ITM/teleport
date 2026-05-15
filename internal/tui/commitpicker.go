package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/pascualchavez/teleport/internal/git"
)

const iconCommit = "󰜘 "

type CommitPicker struct {
	commits  []git.Commit
	selected map[int]bool
	cursor   int
	done     bool
	quitting bool
}

var (
	commitShortStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	commitDateStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func NewCommitPicker(commits []git.Commit) CommitPicker {
	return CommitPicker{
		commits:  commits,
		selected: make(map[int]bool),
	}
}

func (m CommitPicker) Init() tea.Cmd { return nil }

func (m CommitPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.commits)-1 {
				m.cursor++
			}
		case "tab", " ":
			if len(m.commits) > 0 {
				m.selected[m.cursor] = !m.selected[m.cursor]
			}
		case "a":
			allOn := true
			for i := range m.commits {
				if !m.selected[i] {
					allOn = false
					break
				}
			}
			for i := range m.commits {
				m.selected[i] = !allOn
			}
		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m CommitPicker) View() tea.View {
	if m.quitting || m.done {
		return tea.NewView("")
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("  Local commits ahead of upstream") + "\n\n")

	for i, c := range m.commits {
		prefix := "    "
		if i == m.cursor {
			prefix = "  ▶ "
		}

		var mark string
		if m.selected[i] {
			mark = checkStyle.Render(iconChecked)
		} else {
			mark = uncheckedStyle.Render("󰄱 ")
		}

		line := fmt.Sprintf("%s%s%s  %s  %s",
			prefix,
			mark,
			commitShortStyle.Render(c.Short),
			c.Subject,
			commitDateStyle.Render(c.RelDate),
		)
		b.WriteString(line + "\n")
	}

	b.WriteString("\n" + dimStyle.Render("  tab=toggle  a=all  enter=confirm  ctrl+c=quit") + "\n")
	return tea.NewView(b.String())
}

func (m CommitPicker) SelectedCommits() []git.Commit {
	out := make([]git.Commit, 0, len(m.selected))
	for i, c := range m.commits {
		if m.selected[i] {
			out = append(out, c)
		}
	}
	return out
}

func RunCommitPicker(commits []git.Commit) ([]git.Commit, error) {
	if len(commits) == 0 {
		return nil, nil
	}
	p := tea.NewProgram(NewCommitPicker(commits))
	m, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("commit picker: %w", err)
	}
	result := m.(CommitPicker)
	if result.quitting {
		return nil, fmt.Errorf("cancelled")
	}
	return result.SelectedCommits(), nil
}
