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
	lipgloss.NewStyle().Foreground(lipgloss.Color("45")),  // cyan
	lipgloss.NewStyle().Foreground(lipgloss.Color("43")),  // teal
	lipgloss.NewStyle().Foreground(lipgloss.Color("81")),  // sky
	lipgloss.NewStyle().Foreground(lipgloss.Color("220")), // gold
	lipgloss.NewStyle().Foreground(lipgloss.Color("215")), // light orange
	lipgloss.NewStyle().Foreground(lipgloss.Color("208")), // orange
	lipgloss.NewStyle().Foreground(lipgloss.Color("209")), // salmon
	lipgloss.NewStyle().Foreground(lipgloss.Color("205")), // pink
	lipgloss.NewStyle().Foreground(lipgloss.Color("213")), // light magenta
	lipgloss.NewStyle().Foreground(lipgloss.Color("199")), // deep pink
	lipgloss.NewStyle().Foreground(lipgloss.Color("171")), // magenta
	lipgloss.NewStyle().Foreground(lipgloss.Color("141")), // purple
	lipgloss.NewStyle().Foreground(lipgloss.Color("99")),  // violet
	lipgloss.NewStyle().Foreground(lipgloss.Color("147")), // periwinkle
	lipgloss.NewStyle().Foreground(lipgloss.Color("105")), // indigo
}

// beamRange is the contiguous [lo, hi) slice of m.changes that belongs to one
// commit, used by the per-commit filter view.
type beamRange struct{ lo, hi int }

type BeamFilePicker struct {
	changes  []git.FileChange
	selected map[int]bool
	cursor   int
	height   int
	width    int
	done     bool
	quitting bool

	selectedN  int                       // commits selected upstream (may exceed len(legend))
	legend     []git.Commit              // contributing commits, in display order
	shaStyle   map[string]lipgloss.Style // commit SHA → accent style
	shortBySHA map[string]string         // commit SHA → short SHA (for the viewer header)
	ranges     []beamRange               // per-legend-index file range in m.changes
	filter     int                       // -1 = all commits, else index into legend

	load    FileContentFunc // caller-supplied content loader (does the git+highlight I/O)
	viewing bool            // true while the embedded file/diff viewer is open
	viewer  beamFileViewer  // the bat-style pager, valid only while viewing
}

// beamCommitColors walks the commits in display order and assigns a palette
// color to every commit that contributes at least one file in changes. It is
// the single source of truth for the file picker and the send view, so colors
// stay consistent between them. Returns the contributing commits (legend), the
// SHA→display-order index, and the SHA→style map.
func beamCommitColors(changes []git.FileChange, commits []git.Commit) (legend []git.Commit, order map[string]int, shaStyle map[string]lipgloss.Style) {
	present := make(map[string]bool, len(changes))
	for _, c := range changes {
		present[c.SHA] = true
	}

	shaStyle = make(map[string]lipgloss.Style, len(commits))
	order = make(map[string]int, len(commits))
	for _, cm := range commits {
		if !present[cm.SHA] {
			continue
		}
		shaStyle[cm.SHA] = beamCommitPalette[len(legend)%len(beamCommitPalette)]
		order[cm.SHA] = len(legend)
		legend = append(legend, cm)
	}
	return legend, order, shaStyle
}

// BeamFileStyles returns the per-commit accent style keyed by commit SHA, using
// the same assignment as the beam file picker. Pass the full change set the
// picker was built from (not a later subset) so the colors match exactly.
func BeamFileStyles(changes []git.FileChange, commits []git.Commit) map[string]lipgloss.Style {
	_, _, shaStyle := beamCommitColors(changes, commits)
	return shaStyle
}

func NewBeamFilePicker(changes []git.FileChange, commits []git.Commit, load FileContentFunc) BeamFilePicker {
	// Each file's SHA is the commit whose blob will be uploaded. Assign each
	// contributing commit a palette color (the legend stays honest).
	legend, order, shaStyle := beamCommitColors(changes, commits)

	shortBySHA := make(map[string]string, len(legend))
	for _, cm := range legend {
		shortBySHA[cm.SHA] = cm.Short
	}

	// Group files by commit (commit order), alphabetical within each group, so
	// each block of color reads as one commit and each filter is a contiguous
	// slice.
	sorted := make([]git.FileChange, len(changes))
	copy(sorted, changes)
	sort.SliceStable(sorted, func(i, j int) bool {
		if oi, oj := order[sorted[i].SHA], order[sorted[j].SHA]; oi != oj {
			return oi < oj
		}
		return sorted[i].Path < sorted[j].Path
	})

	// Precompute the contiguous file range for each contributing commit.
	ranges := make([]beamRange, len(legend))
	for i := 0; i < len(sorted); {
		ord := order[sorted[i].SHA]
		j := i
		for j < len(sorted) && order[sorted[j].SHA] == ord {
			j++
		}
		ranges[ord] = beamRange{lo: i, hi: j}
		i = j
	}

	selected := make(map[int]bool, len(sorted))
	for i := range sorted {
		selected[i] = true
	}
	return BeamFilePicker{
		changes:    sorted,
		selected:   selected,
		height:     24,
		width:      80,
		selectedN:  len(commits),
		legend:     legend,
		shaStyle:   shaStyle,
		shortBySHA: shortBySHA,
		ranges:     ranges,
		filter:     -1,
		load:       load,
	}
}

func (m BeamFilePicker) Init() tea.Cmd { return nil }

// activeRange returns the [lo, hi) slice of m.changes currently visible given
// the commit filter.
func (m BeamFilePicker) activeRange() (lo, hi int) {
	if m.filter < 0 || m.filter >= len(m.ranges) {
		return 0, len(m.changes)
	}
	return m.ranges[m.filter].lo, m.ranges[m.filter].hi
}

// cycleFilter moves the commit filter by delta (wrapping through the "all"
// pseudo-entry at position 0) and parks the cursor at the new range's start.
func (m *BeamFilePicker) cycleFilter(delta int) {
	if len(m.legend) == 0 {
		return
	}
	total := len(m.legend) + 1 // +1 for the "all" entry
	pos := (m.filter + 1 + delta + total) % total
	m.filter = pos - 1
	lo, _ := m.activeRange()
	m.cursor = lo
}

// loadCmd builds the tea.Cmd that fetches and renders one (file, mode) pair via
// the caller-supplied loader, off the UI thread.
func (m BeamFilePicker) loadCmd(fc git.FileChange, mode ViewerMode) tea.Cmd {
	load := m.load
	width := m.width
	return func() tea.Msg {
		c, err := load(fc, mode, width)
		return viewerContentMsg{mode: mode, content: c, err: err}
	}
}

// openViewer opens the embedded pager for the file under the cursor.
func (m BeamFilePicker) openViewer(mode ViewerMode) (tea.Model, tea.Cmd) {
	c := m.changes[m.cursor]
	m.viewer = newBeamFileViewer(c, m.shortBySHA[c.SHA], m.shaStyle[c.SHA], mode, m.width, m.height)
	m.viewing = true
	return m, m.loadCmd(c, mode)
}

// updateViewing handles keys while the pager is open. esc/q close it (back to
// the picker, state intact); tab switches file⇄diff; everything else scrolls.
func (m BeamFilePicker) updateViewing(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "q":
		m.viewing = false
		return m, nil
	case "tab":
		other := ViewFile
		if m.viewer.mode == ViewFile {
			other = ViewDiff
		}
		m.viewer.mode = other
		if m.viewer.loaded[other] {
			m.viewer.show(other)
			return m, nil
		}
		m.viewer.loading = true
		m.viewer.err = nil
		return m, m.loadCmd(m.viewer.fc, other)
	default:
		var cmd tea.Cmd
		m.viewer, cmd = m.viewer.Update(msg)
		return m, cmd
	}
}

func (m BeamFilePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		if m.viewing {
			m.viewer.resize(msg.Width, msg.Height)
		}
	case viewerContentMsg:
		m.viewer.setContent(msg.mode, msg.content, msg.err)
	case tea.KeyPressMsg:
		if m.viewing {
			return m.updateViewing(msg)
		}
		lo, hi := m.activeRange()
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > lo {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < hi-1 {
				m.cursor++
			}
		case "left", "h":
			m.cycleFilter(-1)
		case "right", "l":
			m.cycleFilter(1)
		case "tab", " ":
			if hi > lo {
				m.selected[m.cursor] = !m.selected[m.cursor]
			}
		case "v":
			if hi > lo {
				return m.openViewer(ViewFile)
			}
		case "d":
			if hi > lo {
				return m.openViewer(ViewDiff)
			}
		case "a":
			// Toggle every file in the active filter as a group.
			allOn := true
			for i := lo; i < hi; i++ {
				if !m.selected[i] {
					allOn = false
					break
				}
			}
			for i := lo; i < hi; i++ {
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
	if m.viewing {
		return tea.NewView(m.viewer.View())
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("  Files from selected commits") + "\n")

	// Status line: in "all" mode a count + filter hint; in filtered mode the
	// active commit (colored cube + short SHA + subject + position).
	if m.filter < 0 {
		// selectedN counts the commits chosen upstream; len(legend) counts those
		// that own a winning (deduped) file. When older commits are fully
		// superseded the two differ — surface both so the gap is explicit, not a
		// silent drop.
		count := fmt.Sprintf("%d commits", m.selectedN)
		if m.selectedN > len(m.legend) {
			count = fmt.Sprintf("%d commits · %d with files", m.selectedN, len(m.legend))
		}
		b.WriteString(dimStyle.Render("  "+count+"  ◂ ←/→ filter by commit ▸") + "\n")
	} else {
		cm := m.legend[m.filter]
		style := m.shaStyle[cm.SHA]
		subj := truncate(cm.Subject, m.width-32)
		pos := dimStyle.Render(fmt.Sprintf("   ◂ %d/%d ▸", m.filter+1, len(m.legend)))
		b.WriteString("  " + style.Render(iconCube+cm.Short) + "  " + subj + pos + "\n")
	}
	b.WriteString("\n")

	// chrome: header(1) + status(1) + blank(1) + blank(1) + footer(1) = 5 lines.
	lo, hi := m.activeRange()
	n := hi - lo
	win := computeWindow(n, m.cursor-lo, m.height-5)
	if h := scrollUpHint(win.above); h != "" {
		b.WriteString(h + "\n")
	}
	for k := win.start; k < win.end; k++ {
		i := lo + k
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

	b.WriteString("\n" + dimStyle.Render("  tab=toggle  a=all  ←/→=commit  v=view  d=diff  enter=confirm  ctrl+c=quit") + "\n")
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

func RunBeamFilePicker(changes []git.FileChange, commits []git.Commit, load FileContentFunc) ([]git.FileChange, error) {
	if len(changes) == 0 {
		return nil, nil
	}
	p := tea.NewProgram(NewBeamFilePicker(changes, commits, load))
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
