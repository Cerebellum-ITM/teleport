package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type localEntry struct {
	name  string
	isDir bool
}

type LocalFilePicker struct {
	cwd      string
	header   string
	entries  []localEntry
	filter   textinput.Model
	cursor   int
	chosen   string
	quitting bool
	err      error
}

func NewLocalFilePicker(startPath, header string) LocalFilePicker {
	fi := textinput.New()
	fi.Placeholder = "filter..."
	fi.Focus()

	m := LocalFilePicker{
		cwd:    startPath,
		header: header,
		filter: fi,
	}
	m.entries = m.readEntries(startPath)
	return m
}

func (m LocalFilePicker) readEntries(dir string) []localEntry {
	raw, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []localEntry
	for _, e := range raw {
		out = append(out, localEntry{name: e.Name(), isDir: e.IsDir()})
	}
	return out
}

func (m LocalFilePicker) filteredEntries() []localEntry {
	q := strings.ToLower(m.filter.Value())
	if q == "" {
		return m.entries
	}
	var out []localEntry
	for _, e := range m.entries {
		if strings.Contains(strings.ToLower(e.name), q) {
			out = append(out, e)
		}
	}
	return out
}

func (m LocalFilePicker) Init() tea.Cmd { return nil }

func (m LocalFilePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	visible := m.filteredEntries()

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
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
			if len(visible) == 0 {
				return m, nil
			}
			sel := visible[m.cursor]
			if sel.isDir {
				// Descend into directory.
				newPath := filepath.Join(m.cwd, sel.name)
				m.cwd = newPath
				m.entries = m.readEntries(newPath)
				m.cursor = 0
				m.filter.SetValue("")
				return m, nil
			}
			// File selected.
			m.chosen = filepath.Join(m.cwd, sel.name)
			return m, tea.Quit

		case "tab", "right":
			if len(visible) > 0 && visible[m.cursor].isDir {
				newPath := filepath.Join(m.cwd, visible[m.cursor].name)
				m.cwd = newPath
				m.entries = m.readEntries(newPath)
				m.cursor = 0
				m.filter.SetValue("")
				return m, nil
			}
			return m, nil

		case "shift+tab", "left", "backspace":
			if m.filter.Value() == "" {
				parent := filepath.Dir(m.cwd)
				if parent != m.cwd {
					m.cwd = parent
					m.entries = m.readEntries(parent)
					m.cursor = 0
				}
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	if m.cursor >= len(m.filteredEntries()) {
		m.cursor = 0
	}
	return m, cmd
}

func (m LocalFilePicker) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render(m.header) + "\n")
	b.WriteString(dimStyle.Render("  "+m.cwd) + "\n")
	b.WriteString("  " + m.filter.View() + "\n\n")

	visible := m.filteredEntries()

	if len(visible) == 0 {
		if m.filter.Value() != "" {
			b.WriteString(dimStyle.Render("  (no matches)") + "\n")
		} else {
			b.WriteString(dimStyle.Render("  (empty directory)") + "\n")
		}
	}

	for i, e := range visible {
		icon := "  "
		if e.isDir {
			icon = iconFolder
		} else {
			icon = fileTypeIcon(e.name) + " "
		}
		label := icon + e.name
		if i == m.cursor {
			b.WriteString(cursorStyle.Render("  ▶ "+label) + "\n")
		} else {
			b.WriteString("    " + dimStyle.Render(label) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  ↑↓ navigate  tab/→=descend  ←=up  enter=select file  esc=clear filter  q=quit") + "\n")

	return tea.NewView(b.String())
}

// RunLocalFilePicker opens an interactive file browser on the local filesystem
// starting at startPath. Returns the selected file path or an error.
func RunLocalFilePicker(startPath, header string) (string, error) {
	if startPath == "" {
		startPath = "."
	}
	p := tea.NewProgram(NewLocalFilePicker(startPath, header))
	m, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("file picker: %w", err)
	}
	result := m.(LocalFilePicker)
	if result.quitting || result.chosen == "" {
		return "", fmt.Errorf("no file selected")
	}
	return result.chosen, nil
}
