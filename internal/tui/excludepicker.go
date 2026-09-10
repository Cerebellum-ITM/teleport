package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// ErrPickerCancelled reports that the picker was closed without saving.
var ErrPickerCancelled = errors.New("cancelled")

// ExcludePicker browses a local tree and toggles entries at any depth. Unlike
// LocalFilePicker it is multi-select and can mark a directory itself, which is
// the point: the paths worth excluding from a push are usually directories, and
// they are rarely at the top level.
type ExcludePicker struct {
	root     string
	cwd      string
	header   string
	entries  []localEntry
	selected map[string]bool
	// visited holds every path this session rendered, so the caller can tell an
	// entry that was deliberately left unchecked from one never shown at all.
	visited  map[string]bool
	cursor   int
	height   int
	quitting bool
}

func NewExcludePicker(root, header string, preselected []string) ExcludePicker {
	selected := make(map[string]bool, len(preselected))
	for _, p := range preselected {
		selected[p] = true
	}

	m := ExcludePicker{
		root:     root,
		cwd:      root,
		header:   header,
		selected: selected,
		visited:  make(map[string]bool),
		height:   24,
	}
	m.load(root)
	return m
}

// load reads dir and records its entries as visited.
func (m *ExcludePicker) load(dir string) {
	m.cwd = dir
	m.entries = nil
	raw, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range raw {
		m.entries = append(m.entries, localEntry{name: e.Name(), isDir: e.IsDir()})
		m.visited[m.rel(e.Name())] = true
	}
}

// rel is the path of an entry of the current directory relative to the root, in
// slash form: the pattern that excluding it writes.
func (m ExcludePicker) rel(name string) string {
	r, err := filepath.Rel(m.root, filepath.Join(m.cwd, name))
	if err != nil {
		return name
	}
	return filepath.ToSlash(r)
}

func (m ExcludePicker) Init() tea.Cmd { return nil }

func (m ExcludePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}

		case "tab", " ", "x":
			if len(m.entries) > 0 {
				p := m.rel(m.entries[m.cursor].name)
				m.selected[p] = !m.selected[p]
			}

		case "right", "l":
			if len(m.entries) > 0 && m.entries[m.cursor].isDir {
				m.load(filepath.Join(m.cwd, m.entries[m.cursor].name))
				m.cursor = 0
			}

		case "left", "h", "backspace":
			if m.cwd != m.root {
				child := m.cwd
				m.load(filepath.Dir(m.cwd))
				m.cursor = 0
				for i, e := range m.entries {
					if filepath.Join(m.cwd, e.name) == child {
						m.cursor = i
						break
					}
				}
			}

		case "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ExcludePicker) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render(m.header) + "\n")
	here := "."
	if rel, err := filepath.Rel(m.root, m.cwd); err == nil {
		here = filepath.ToSlash(rel)
	}
	b.WriteString(dimStyle.Render("  "+here) + "\n\n")

	if len(m.entries) == 0 {
		b.WriteString(dimStyle.Render("  (empty directory)") + "\n")
	}

	// chrome: header(1) + path(1) + blank(1) + blank(1) + footer(1) = 5 lines.
	win := computeWindow(len(m.entries), m.cursor, m.height-5)
	if h := scrollUpHint(win.above); h != "" {
		b.WriteString(h + "\n")
	}
	for i := win.start; i < win.end; i++ {
		e := m.entries[i]
		name := e.name
		if e.isDir {
			name += "/"
		}
		prefix := "    "
		if i == m.cursor {
			prefix = "  ▶ "
		}
		if m.selected[m.rel(e.name)] {
			b.WriteString(prefix + checkStyle.Render(iconChecked+name) + "\n")
		} else {
			b.WriteString(prefix + uncheckedStyle.Render("󰄱 "+name) + "\n")
		}
	}
	if h := scrollDownHint(win.below); h != "" {
		b.WriteString(h + "\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  ↑↓ navigate  →=enter folder  ←=up  tab=toggle  enter=save  q=cancel") + "\n")
	return tea.NewView(b.String())
}

// RunExcludePicker browses root and returns the paths toggled on, plus every
// path it showed. The second list is what lets a caller drop an entry the user
// unchecked without touching patterns for directories that were never opened.
// Cancelling returns ErrPickerCancelled.
func RunExcludePicker(root, header string, preselected []string) (picks, visited []string, err error) {
	if root == "" {
		root = "."
	}
	p := tea.NewProgram(NewExcludePicker(root, header, preselected))
	m, err := p.Run()
	if err != nil {
		return nil, nil, fmt.Errorf("exclude picker: %w", err)
	}

	result := m.(ExcludePicker)
	if result.quitting {
		return nil, nil, ErrPickerCancelled
	}
	for p, on := range result.selected {
		if on {
			picks = append(picks, p)
		}
	}
	for p := range result.visited {
		visited = append(visited, p)
	}
	return picks, visited, nil
}
