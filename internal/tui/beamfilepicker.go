package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/pascualchavez/teleport/internal/git"
)

const iconDelete = "󰮈 "

var deleteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))

type BeamFilePicker struct {
	changes  []git.FileChange
	selected map[int]bool
	cursor   int
	done     bool
	quitting bool
}

func NewBeamFilePicker(changes []git.FileChange) BeamFilePicker {
	selected := make(map[int]bool, len(changes))
	for i := range changes {
		selected[i] = true
	}
	return BeamFilePicker{changes: changes, selected: selected}
}

func (m BeamFilePicker) Init() tea.Cmd { return nil }

func (m BeamFilePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.cursor < len(m.changes)-1 {
				m.cursor++
			}
		case "tab", " ":
			if len(m.changes) > 0 {
				m.selected[m.cursor] = !m.selected[m.cursor]
			}
		case "a":
			allOn := true
			for i := range m.changes {
				if !m.selected[i] {
					allOn = false
					break
				}
			}
			for i := range m.changes {
				m.selected[i] = !allOn
			}
		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m BeamFilePicker) View() tea.View {
	if m.quitting || m.done {
		return tea.NewView("")
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("  Files from selected commits") + "\n\n")

	for i, c := range m.changes {
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

		var label string
		switch c.Status {
		case 'D':
			label = deleteStyle.Render(iconDelete + c.Path + "  (delete)")
		default:
			label = iconFile + c.Path
		}

		b.WriteString(prefix + mark + label + "\n")
	}

	b.WriteString("\n" + dimStyle.Render("  tab=toggle  a=all  enter=confirm  ctrl+c=quit") + "\n")
	return tea.NewView(b.String())
}

func (m BeamFilePicker) SelectedChanges() []git.FileChange {
	out := make([]git.FileChange, 0, len(m.selected))
	for i, c := range m.changes {
		if m.selected[i] {
			out = append(out, c)
		}
	}
	return out
}

func RunBeamFilePicker(changes []git.FileChange) ([]git.FileChange, error) {
	if len(changes) == 0 {
		return nil, nil
	}
	p := tea.NewProgram(NewBeamFilePicker(changes))
	m, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("beam file picker: %w", err)
	}
	result := m.(BeamFilePicker)
	if result.quitting {
		return nil, fmt.Errorf("cancelled")
	}
	return result.SelectedChanges(), nil
}
