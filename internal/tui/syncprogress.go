package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// SyncFileDone is sent by the uploader goroutine after each file completes.
type SyncFileDone struct {
	Path string
	Err  error
}

type syncTickMsg time.Time

var (
	spOKStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	spErrStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	spSepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	spBarStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("116"))
	spStatsStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	spHeaderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	spIconStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("116"))
)

type SyncProgress struct {
	header string
	done   []SyncFileDone
	total  int
	start  time.Time
	width  int
	height int
}

func NewSyncProgress(header string, total int) SyncProgress {
	return SyncProgress{
		header: header,
		total:  total,
		start:  time.Now(),
		width:  80,
		height: 24,
	}
}

func (m SyncProgress) Init() tea.Cmd {
	return syncTick()
}

func syncTick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return syncTickMsg(t)
	})
}

func (m SyncProgress) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case SyncFileDone:
		m.done = append(m.done, msg)
		if len(m.done) == m.total {
			return m, tea.Quit
		}
	case syncTickMsg:
		return m, syncTick()
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m SyncProgress) View() tea.View {
	var b strings.Builder

	// 3 lines: empty + header + empty
	// 3 lines: sep + bar + sep
	// remainder: log area
	logAreaHeight := m.height - 6
	if logAreaHeight < 1 {
		logAreaHeight = 1
	}

	// Header block
	fmt.Fprintf(&b, "\n  %s\n\n", spHeaderStyle.Render(m.header))

	// Log area — show last logAreaHeight entries, pad top with blank lines
	startIdx := 0
	if len(m.done) > logAreaHeight {
		startIdx = len(m.done) - logAreaHeight
	}
	visible := m.done[startIdx:]

	for i := 0; i < logAreaHeight-len(visible); i++ {
		b.WriteString("\n")
	}
	for _, f := range visible {
		icon := spIconStyle.Render(fileTypeIcon(f.Path))
		if f.Err != nil {
			fmt.Fprintf(&b, "  %s %s %s\n", spErrStyle.Render("✗"), icon, spErrStyle.Render(f.Path))
		} else {
			fmt.Fprintf(&b, "  %s %s %s\n", spOKStyle.Render("✓"), icon, f.Path)
		}
	}

	// Bar block — no trailing newline on last sep so bubbletea doesn't add a blank line
	sep := spSepStyle.Render(strings.Repeat("─", m.width))
	fmt.Fprintf(&b, "%s\n%s\n%s", sep, m.renderBar(), sep)

	return tea.NewView(b.String())
}

func (m SyncProgress) renderBar() string {
	done := len(m.done)
	pct := 0
	if m.total > 0 {
		pct = (done * 100) / m.total
	}

	elapsed := time.Since(m.start)
	stats := fmt.Sprintf("  %d/%d  %3d%%  %s  ", done, m.total, pct, formatSyncDuration(elapsed))

	barWidth := m.width - 4 - len(stats) // 4 = "  [" + "]"
	if barWidth < 4 {
		barWidth = 4
	}

	filled := 0
	if m.total > 0 {
		filled = (done * barWidth) / m.total
	}

	var inner strings.Builder
	for i := 0; i < barWidth; i++ {
		switch {
		case i < filled-1:
			inner.WriteString("=")
		case i == filled-1 && done < m.total:
			inner.WriteString(">")
		case i == filled-1 && done == m.total && filled > 0:
			inner.WriteString("=")
		default:
			inner.WriteString(" ")
		}
	}

	return "  [" + spBarStyle.Render(inner.String()) + "]" + spStatsStyle.Render(stats)
}

func formatSyncDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

// fileTypeIcon returns a Nerd Font glyph matching the file's extension.
func fileTypeIcon(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if len(ext) > 1 {
		ext = ext[1:] // strip leading dot
	}
	icons := map[string]string{
		"go":   "",
		"py":   "",
		"js":   "",
		"ts":   "",
		"md":   "",
		"json": "",
		"yaml": "",
		"yml":  "",
		"html": "",
		"css":  "",
		"rs":   "",
	}
	if icon, ok := icons[ext]; ok {
		return icon
	}
	return "" // cod-file fallback
}

// RunSyncProgress runs the progress TUI, uploading each file via the upload
// callback. Returns the number of failed uploads.
func RunSyncProgress(header string, files []string, upload func(string) error) (int, error) {
	model := NewSyncProgress(header, len(files))
	p := tea.NewProgram(model)

	go func() {
		for _, f := range files {
			p.Send(SyncFileDone{Path: f, Err: upload(f)})
		}
	}()

	m, err := p.Run()
	if err != nil {
		return 0, fmt.Errorf("sync progress: %w", err)
	}

	final := m.(SyncProgress)
	failed := 0
	for _, f := range final.done {
		if f.Err != nil {
			failed++
		}
	}
	return failed, nil
}
