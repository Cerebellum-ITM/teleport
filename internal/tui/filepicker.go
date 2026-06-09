package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	iconFile    = "󰈙 "
	iconChecked = "󰱒 "
	iconSync    = "󰒃 "
)

type FilePicker struct {
	tracked   []string
	untracked []string
	selected  map[int]bool // index into untracked
	cursor    int
	height    int
	done      bool
	quitting  bool
}

var (
	trackedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	checkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	uncheckedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

func NewFilePicker(tracked, untracked []string) FilePicker {
	return FilePicker{
		tracked:   tracked,
		untracked: untracked,
		selected:  make(map[int]bool),
		height:    24,
	}
}

func (m FilePicker) Init() tea.Cmd {
	return nil
}

func (m FilePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
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
			if m.cursor < len(m.untracked)-1 {
				m.cursor++
			}

		case " ":
			if len(m.untracked) > 0 {
				m.selected[m.cursor] = !m.selected[m.cursor]
			}

		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m FilePicker) View() tea.View {
	if m.quitting || m.done {
		return tea.NewView("")
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("  Files to Sync") + "\n\n")

	// Split the available height between the two sections. The untracked list
	// is interactive (cursor), so it gets priority; tracked just scrolls from
	// the top. chrome: header(2) + tracked-label(1) + untracked-label-block(2)
	// + footer-block(2) = 7 lines.
	trackedRows, untrackedRows := m.sectionRows()

	b.WriteString(dimStyle.Render("  Tracked (always included):") + "\n")
	twin := computeWindow(len(m.tracked), 0, trackedRows)
	for i := twin.start; i < twin.end; i++ {
		b.WriteString(trackedStyle.Render("    "+iconFile+m.tracked[i]) + "\n")
	}
	if h := scrollDownHint(twin.below); h != "" {
		b.WriteString(h + "\n")
	}

	if len(m.untracked) > 0 {
		b.WriteString("\n" + dimStyle.Render("  Untracked (space to toggle):") + "\n")
		uwin := computeWindow(len(m.untracked), m.cursor, untrackedRows)
		if h := scrollUpHint(uwin.above); h != "" {
			b.WriteString(h + "\n")
		}
		for i := uwin.start; i < uwin.end; i++ {
			f := m.untracked[i]
			prefix := "    "
			if i == m.cursor {
				prefix = "  ▶ "
			}

			if m.selected[i] {
				b.WriteString(prefix + checkStyle.Render(iconChecked+f) + "\n")
			} else {
				b.WriteString(prefix + uncheckedStyle.Render("󰄱 "+f) + "\n")
			}
		}
		if h := scrollDownHint(uwin.below); h != "" {
			b.WriteString(h + "\n")
		}
	}

	b.WriteString("\n" + dimStyle.Render("  space=toggle  enter=confirm  ctrl+c=quit") + "\n")
	return tea.NewView(b.String())
}

// sectionRows divides the available list height between the tracked and
// untracked sections. The untracked (interactive) list is favored; any slack
// from a short section is handed to the other.
func (m FilePicker) sectionRows() (tracked, untracked int) {
	avail := m.height - 7
	if avail < 4 {
		avail = 4
	}
	if len(m.tracked)+len(m.untracked) <= avail {
		return len(m.tracked), len(m.untracked)
	}

	untracked = (avail * 3) / 5
	if untracked < 3 {
		untracked = 3
	}
	tracked = avail - untracked

	switch {
	case len(m.untracked) < untracked:
		untracked = len(m.untracked)
		tracked = avail - untracked
	case len(m.tracked) < tracked:
		tracked = len(m.tracked)
		untracked = avail - tracked
	}
	return tracked, untracked
}

func (m FilePicker) SelectedFiles() []string {
	result := make([]string, len(m.tracked))
	copy(result, m.tracked)
	for i, f := range m.untracked {
		if m.selected[i] {
			result = append(result, f)
		}
	}
	return result
}

func RunFilePicker(tracked, untracked []string) ([]string, error) {
	if len(untracked) == 0 {
		return tracked, nil
	}

	p := tea.NewProgram(NewFilePicker(tracked, untracked))
	m, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("file picker: %w", err)
	}

	result := m.(FilePicker)
	if result.quitting {
		return nil, fmt.Errorf("cancelled")
	}
	return result.SelectedFiles(), nil
}
