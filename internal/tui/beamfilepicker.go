package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/pascualchavez/teleport/internal/git"
)

const (
	iconDelete = "󰮈 "
	iconCube   = "󰆧 "
)

var deleteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))

// beamCommitPalette assigns a distinct accent color to each contributing
// commit so files can be grouped visually by origin. It cycles when there are
// more commits than colors. Documented in context/ui-context.md.
var beamCommitPalette = []lipgloss.Style{
	lipgloss.NewStyle().Foreground(lipgloss.Color("39")),  // blue
	lipgloss.NewStyle().Foreground(lipgloss.Color("170")), // magenta
	lipgloss.NewStyle().Foreground(lipgloss.Color("220")), // gold
	lipgloss.NewStyle().Foreground(lipgloss.Color("141")), // purple
	lipgloss.NewStyle().Foreground(lipgloss.Color("80")),  // cyan
	lipgloss.NewStyle().Foreground(lipgloss.Color("209")), // salmon
	lipgloss.NewStyle().Foreground(lipgloss.Color("43")),  // teal
	lipgloss.NewStyle().Foreground(lipgloss.Color("147")), // periwinkle
}

type BeamFilePicker struct {
	changes  []git.FileChange
	selected map[int]bool
	cursor   int
	height   int
	done     bool
	quitting bool

	legend   []git.Commit              // contributing commits, in display order
	shaStyle map[string]lipgloss.Style // commit SHA → accent style
}

func NewBeamFilePicker(changes []git.FileChange, commits []git.Commit) BeamFilePicker {
	// Each file's SHA is the commit whose blob will be uploaded. Walk the
	// selected commits in display order and assign a palette color to every
	// commit that actually contributes a file (so the legend stays honest).
	present := make(map[string]bool, len(changes))
	for _, c := range changes {
		present[c.SHA] = true
	}

	shaStyle := make(map[string]lipgloss.Style, len(commits))
	order := make(map[string]int, len(commits))
	var legend []git.Commit
	for _, cm := range commits {
		if !present[cm.SHA] {
			continue
		}
		shaStyle[cm.SHA] = beamCommitPalette[len(legend)%len(beamCommitPalette)]
		order[cm.SHA] = len(legend)
		legend = append(legend, cm)
	}

	// Group files by commit (commit order), alphabetical within each group, so
	// each block of color reads as one commit.
	sorted := make([]git.FileChange, len(changes))
	copy(sorted, changes)
	sort.SliceStable(sorted, func(i, j int) bool {
		if oi, oj := order[sorted[i].SHA], order[sorted[j].SHA]; oi != oj {
			return oi < oj
		}
		return sorted[i].Path < sorted[j].Path
	})

	selected := make(map[int]bool, len(sorted))
	for i := range sorted {
		selected[i] = true
	}
	return BeamFilePicker{
		changes:  sorted,
		selected: selected,
		height:   24,
		legend:   legend,
		shaStyle: shaStyle,
	}
}

func (m BeamFilePicker) Init() tea.Cmd { return nil }

func (m BeamFilePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	// Legend: one line per contributing commit — colored cube + short SHA +
	// subject — so each color is tied to a recognizable commit.
	for _, cm := range m.legend {
		style := m.shaStyle[cm.SHA]
		b.WriteString("  " + style.Render(iconCube+cm.Short) + "  " + cm.Subject + "\n")
	}
	if len(m.legend) > 0 {
		b.WriteString("\n")
	}

	// chrome: header(1) + blank(1) + footer-block(2) = 4 lines, plus the
	// legend block (one line per commit + a trailing blank) when present.
	chrome := 4
	if len(m.legend) > 0 {
		chrome += len(m.legend) + 1
	}
	win := computeWindow(len(m.changes), m.cursor, m.height-chrome)
	if h := scrollUpHint(win.above); h != "" {
		b.WriteString(h + "\n")
	}
	for i := win.start; i < win.end; i++ {
		c := m.changes[i]
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

		cstyle := m.shaStyle[c.SHA]
		line := prefix + mark + cstyle.Render(iconCube+c.Path)
		if c.Status == 'D' {
			line += deleteStyle.Render("  (delete)")
		}
		b.WriteString(line + "\n")
	}
	if h := scrollDownHint(win.below); h != "" {
		b.WriteString(h + "\n")
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

func RunBeamFilePicker(changes []git.FileChange, commits []git.Commit) ([]git.FileChange, error) {
	if len(changes) == 0 {
		return nil, nil
	}
	p := tea.NewProgram(NewBeamFilePicker(changes, commits))
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
