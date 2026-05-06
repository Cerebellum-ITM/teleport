package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
)

const (
	iconFolder   = "󰉋 "
	iconFolderUp = "󰉋 "
)

type listDirsMsg struct {
	dirs []string
	path string
}

type listDirsErrMsg struct{ err error }

func listDirsCmd(client *sshpkg.Client, path string) tea.Cmd {
	return func() tea.Msg {
		dirs, err := client.ListDirs(path)
		if err != nil {
			return listDirsErrMsg{err}
		}
		return listDirsMsg{dirs: dirs, path: path}
	}
}

type DirPicker struct {
	client   *sshpkg.Client
	cwd      string
	dirs     []string
	filter   textinput.Model
	cursor   int
	chosen   string
	quitting bool
	loading  bool
	err      error
}

var (
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
)

func NewDirPicker(client *sshpkg.Client, startPath string) DirPicker {
	fi := textinput.New()
	fi.Placeholder = "filter..."
	fi.Focus()

	return DirPicker{
		client:  client,
		cwd:     startPath,
		loading: true,
		filter:  fi,
	}
}

func (m DirPicker) Init() tea.Cmd {
	return listDirsCmd(m.client, m.cwd)
}

// filteredDirs returns the subset of m.dirs that match the current filter text.
func (m DirPicker) filteredDirs() []string {
	q := strings.ToLower(m.filter.Value())
	if q == "" {
		return m.dirs
	}
	var out []string
	for _, d := range m.dirs {
		if strings.Contains(strings.ToLower(d), q) {
			out = append(out, d)
		}
	}
	return out
}

func (m DirPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case listDirsMsg:
		m.loading = false
		m.dirs = msg.dirs
		m.cursor = 0
		return m, nil

	case listDirsErrMsg:
		m.loading = false
		m.err = msg.err
		return m, nil

	case tea.KeyPressMsg:
		visible := m.filteredDirs()

		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit

		case "esc":
			m.filter.SetValue("")
			m.cursor = 0
			return m, nil

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j":
			if m.cursor < len(visible)-1 {
				m.cursor++
			}
			return m, nil

		case "enter":
			// Confirm selection of the current directory.
			m.chosen = m.cwd
			return m, tea.Quit

		case "tab", "right":
			// Descend into the highlighted directory.
			if len(visible) > 0 {
				newPath := filepath.Join(m.cwd, visible[m.cursor])
				m.cwd = newPath
				m.loading = true
				m.cursor = 0
				m.filter.SetValue("")
				return m, listDirsCmd(m.client, newPath)
			}
			return m, nil

		case "shift+tab", "left":
			// Go up to the parent directory.
			if m.filter.Value() == "" {
				parent := filepath.Dir(m.cwd)
				if parent != m.cwd {
					m.cwd = parent
					m.loading = true
					m.cursor = 0
					return m, listDirsCmd(m.client, parent)
				}
				return m, nil
			}

		case "backspace":
			// When the filter is already empty, navigate up one level.
			if m.filter.Value() == "" {
				parent := filepath.Dir(m.cwd)
				if parent != m.cwd {
					m.cwd = parent
					m.loading = true
					m.cursor = 0
					return m, listDirsCmd(m.client, parent)
				}
				return m, nil
			}
		}
	}

	// All other keys go to the filter input.
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)

	// Reset cursor when filter text changes so it doesn't go out of range.
	if m.cursor >= len(m.filteredDirs()) {
		m.cursor = 0
	}

	return m, cmd
}

func (m DirPicker) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var b strings.Builder

	b.WriteString(headerStyle.Render("  Remote Directory Browser") + "\n")
	b.WriteString(dimStyle.Render("  "+m.cwd) + "\n")
	b.WriteString("  " + m.filter.View() + "\n\n")

	if m.loading {
		b.WriteString(dimStyle.Render("  Loading...") + "\n")
		return tea.NewView(b.String())
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", m.err))
		return tea.NewView(b.String())
	}

	visible := m.filteredDirs()

	if len(visible) == 0 {
		if m.filter.Value() != "" {
			b.WriteString(dimStyle.Render("  (no matches)") + "\n")
		} else {
			b.WriteString(dimStyle.Render("  (empty directory)") + "\n")
		}
	}

	for i, d := range visible {
		if i == m.cursor {
			b.WriteString(cursorStyle.Render("  ▶ "+iconFolder+d) + "\n")
		} else {
			b.WriteString("    " + dimStyle.Render(iconFolder+d) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  ↑/↓ navigate  tab/→=descend  shift+tab/←=up  enter=select  esc=clear filter  q=quit") + "\n")
	b.WriteString(selectedStyle.Render("  Selected: "+m.cwd) + "\n")

	return tea.NewView(b.String())
}

func RunDirPicker(client *sshpkg.Client, startPath string) (string, error) {
	p := tea.NewProgram(NewDirPicker(client, startPath))
	m, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("dir picker: %w", err)
	}

	result := m.(DirPicker)
	if result.quitting || result.chosen == "" {
		return "", fmt.Errorf("no directory selected")
	}
	return result.chosen, nil
}
