package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var boldStyle = lipgloss.NewStyle().Bold(true)

type branchPickerModel struct {
	branches []string
	current  string
	cursor   int
	chosen   string
	quitting bool
}

func RunBranchPicker(branches []string, current string) (string, error) {
	m := branchPickerModel{
		branches: branches,
		current:  current,
	}
	for i, b := range branches {
		if b == current {
			m.cursor = i
			break
		}
	}

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	final := finalModel.(branchPickerModel)
	if final.quitting {
		return "", nil
	}
	return final.chosen, nil
}

func (m branchPickerModel) Init() tea.Cmd { return nil }

func (m branchPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.cursor < len(m.branches)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.branches) > 0 {
				m.chosen = m.branches[m.cursor]
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m branchPickerModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("  Select source branch") + "\n\n")

	for i, branch := range m.branches {
		prefix := "    "
		if i == m.cursor {
			prefix = "  ▶ "
		}

		line := prefix + boldStyle.Render(branch)
		if branch == m.current {
			line += "  " + dimStyle.Render("(current)")
		}

		b.WriteString(line + "\n")
	}

	b.WriteString("\n" + dimStyle.Render("  ↑↓=navigate  enter=confirm  ctrl+c=quit") + "\n")
	return tea.NewView(b.String())
}
