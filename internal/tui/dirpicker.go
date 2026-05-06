package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
)

const (
	iconFolder  = "󰉋 "
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
	return DirPicker{
		client:  client,
		cwd:     startPath,
		loading: true,
	}
}

func (m DirPicker) Init() tea.Cmd {
	return listDirsCmd(m.client, m.cwd)
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
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.dirs)-1 {
				m.cursor++
			}

		case "enter":
			if len(m.dirs) > 0 {
				newPath := filepath.Join(m.cwd, m.dirs[m.cursor])
				m.cwd = newPath
				m.loading = true
				m.cursor = 0
				return m, listDirsCmd(m.client, newPath)
			}

		case "backspace", "left", "h":
			parent := filepath.Dir(m.cwd)
			if parent != m.cwd {
				m.cwd = parent
				m.loading = true
				m.cursor = 0
				return m, listDirsCmd(m.client, parent)
			}

		case "s":
			m.chosen = m.cwd
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m DirPicker) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var b strings.Builder

	b.WriteString(headerStyle.Render("  Remote Directory Browser") + "\n")
	b.WriteString(dimStyle.Render("  "+m.cwd) + "\n\n")

	if m.loading {
		b.WriteString(dimStyle.Render("  Loading...") + "\n")
		return tea.NewView(b.String())
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", m.err))
		return tea.NewView(b.String())
	}

	if len(m.dirs) == 0 {
		b.WriteString(dimStyle.Render("  (empty directory)") + "\n")
	}

	for i, d := range m.dirs {
		if i == m.cursor {
			b.WriteString(cursorStyle.Render("  ▶ "+iconFolder+d) + "\n")
		} else {
			b.WriteString("    " + dimStyle.Render(iconFolder+d) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  ↑/↓ navigate  enter=enter  backspace=up  s=select this dir  q=quit") + "\n")
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
