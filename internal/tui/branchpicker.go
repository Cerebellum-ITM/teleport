package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	iconBranch = "󰘬 "
	iconMain   = "󰋜 "
)

var boldStyle = lipgloss.NewStyle().Bold(true)

type branchPickerModel struct {
	branches []string
	current  string
	filter   textinput.Model
	cursor   int
	chosen   string
	quitting bool
}

func RunBranchPicker(branches []string, current string) (string, error) {
	fi := textinput.New()
	fi.Placeholder = "filter..."
	fi.Focus()

	m := branchPickerModel{
		branches: branches,
		current:  current,
		filter:   fi,
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

func (m branchPickerModel) Init() tea.Cmd { return textinput.Blink }

func (m branchPickerModel) filtered() []string {
	q := strings.ToLower(m.filter.Value())
	if q == "" {
		return m.branches
	}
	var out []string
	for _, b := range m.branches {
		if strings.Contains(strings.ToLower(b), q) {
			out = append(out, b)
		}
	}
	return out
}

func (m branchPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		visible := m.filtered()
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			m.filter.SetValue("")
			m.cursor = 0
			return m, nil
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "ctrl+n":
			if m.cursor < len(visible)-1 {
				m.cursor++
			}
			return m, nil
		case "enter":
			if len(visible) > 0 {
				m.chosen = visible[m.cursor]
			}
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)

	if m.cursor >= len(m.filtered()) {
		m.cursor = 0
	}
	return m, cmd
}

func branchIcon(name string) string {
	if name == "main" || name == "master" {
		return iconMain
	}
	return iconBranch
}

func (m branchPickerModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("  Select source branch") + "\n")
	b.WriteString("  " + m.filter.View() + "\n\n")

	visible := m.filtered()
	if len(visible) == 0 {
		b.WriteString(dimStyle.Render("  (no matches)") + "\n")
	}

	for i, branch := range visible {
		prefix := "    "
		if i == m.cursor {
			prefix = "  ▶ "
		}

		line := prefix + branchIcon(branch) + boldStyle.Render(branch)
		if branch == m.current {
			line += "  " + dimStyle.Render("(current)")
		}

		b.WriteString(line + "\n")
	}

	b.WriteString("\n" + dimStyle.Render("  type=filter  ↑↓=navigate  enter=confirm  esc=clear  ctrl+c=quit") + "\n")
	return tea.NewView(b.String())
}
