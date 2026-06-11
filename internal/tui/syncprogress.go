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

// BeamGroup is one commit's section in the beam send view: a colored header
// (cube + short SHA + subject) followed by the files that commit contributed.
type BeamGroup struct {
	Style   lipgloss.Style
	Short   string
	Subject string
	Paths   []string
}

type SyncProgress struct {
	header string
	done   []SyncFileDone
	total  int
	start  time.Time
	width  int
	height int
	groups []BeamGroup // when set, files are rendered grouped by commit (beam)
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

	// Log area
	if len(m.groups) > 0 {
		b.WriteString(m.renderGrouped(logAreaHeight))
	} else {
		b.WriteString(m.renderLog(logAreaHeight))
	}

	// Bar block — no trailing newline on last sep so bubbletea doesn't add a blank line
	sep := spSepStyle.Render(strings.Repeat("─", m.width))
	fmt.Fprintf(&b, "%s\n%s\n%s", sep, m.renderBar(), sep)

	return tea.NewView(b.String())
}

// renderLog is the flat streaming log used by plain sync: last completed files
// at the bottom, top padded with blanks.
func (m SyncProgress) renderLog(logAreaHeight int) string {
	var b strings.Builder
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
	return b.String()
}

// renderGrouped is the beam send view: each commit is a colored header followed
// by its files, each marked ✓ done / ✗ failed / · pending. The visible window
// follows the most recently completed file so progress stays on screen.
func (m SyncProgress) renderGrouped(logAreaHeight int) string {
	res := make(map[string]error, len(m.done))
	seen := make(map[string]bool, len(m.done))
	for _, d := range m.done {
		res[d.Path] = d.Err
		seen[d.Path] = true
	}

	var lines []string
	lastActive := 0
	for _, g := range m.groups {
		head := g.Style.Render(iconCube+"["+g.Short+"]") + " " +
			spSepStyle.Render("─") + " " + spHeaderStyle.Render(g.Subject)
		lines = append(lines, "  "+head)
		for _, p := range g.Paths {
			icon := spIconStyle.Render(fileTypeIcon(p))
			var status, name string
			switch {
			case seen[p] && res[p] != nil:
				status, name = spErrStyle.Render("✗"), spErrStyle.Render(p)
				lastActive = len(lines)
			case seen[p]:
				status, name = spOKStyle.Render("✓"), p
				lastActive = len(lines)
			default:
				status, name = spSepStyle.Render("·"), spSepStyle.Render(p)
			}
			lines = append(lines, fmt.Sprintf("      %s %s %s", status, icon, name))
		}
	}

	// Window: end just past the last completed line so it stays visible.
	end := lastActive + 1
	if end < logAreaHeight {
		end = logAreaHeight
	}
	if end > len(lines) {
		end = len(lines)
	}
	start := end - logAreaHeight
	if start < 0 {
		start = 0
	}
	visible := lines[start:end]

	var b strings.Builder
	for i := 0; i < logAreaHeight-len(visible); i++ {
		b.WriteString("\n")
	}
	for _, l := range visible {
		b.WriteString(l + "\n")
	}
	return b.String()
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

// fileTypeIcon returns a Nerd Font glyph matching the file's extension
// (or full basename for files like Dockerfile / Makefile / .gitignore).
func fileTypeIcon(path string) string {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "dockerfile":
		return ""
	case "makefile":
		return ""
	case ".gitignore", ".gitattributes", ".gitmodules":
		return ""
	}

	ext := strings.ToLower(filepath.Ext(path))
	if len(ext) > 1 {
		ext = ext[1:] // strip leading dot
	}
	icons := map[string]string{
		// originals
		"go":   "",
		"py":   "",
		"js":   "",
		"ts":   "",
		"md":   "",
		"json": "",
		"yaml": "",
		"yml":  "",
		"html": "",
		"css":  "",
		"rs":   "",

		// markup / config / shell
		"xml":  "",
		"svg":  "",
		"toml": "",
		"ini":  "",
		"env":  "󰒓",
		"conf": "",
		"cfg":  "",
		"lock": "󰈡",
		"sh":   "",
		"bash": "",
		"zsh":  "",
		"fish": "",
		"ps1":  "󰨊",
		"bat":  "",

		// frontend
		"jsx":    "",
		"tsx":    "",
		"vue":    "󰡄",
		"svelte": "",
		"scss":   "",
		"sass":   "",
		"less":   "",

		// languages
		"c":     "",
		"h":     "",
		"cpp":   "",
		"cc":    "",
		"hpp":   "",
		"java":  "",
		"kt":    "󱈙",
		"swift": "󰛥",
		"rb":    "",
		"php":   "",
		"lua":   "",
		"dart":  "",
		"ex":    "",
		"exs":   "",

		// data
		"sql":    "󰆼",
		"csv":    "",
		"tsv":    "",
		"db":     "󰆼",
		"sqlite": "󰆼",

		// text / logs
		"txt": "󰈚",
		"log": "",
		"rst": "󰧮",

		// images
		"png":  "󰈟",
		"jpg":  "󰈟",
		"jpeg": "󰈟",
		"gif":  "󰈟",
		"webp": "󰈟",
		"ico":  "󰈟",
		"bmp":  "󰈟",

		// archives / binaries
		"zip": "󰗄",
		"tar": "󰗄",
		"gz":  "󰗄",
		"tgz": "󰗄",
		"7z":  "󰗄",
		"rar": "󰗄",
		"pdf": "󰈦",
		"exe": "󰣆",
		"bin": "",
	}
	if icon, ok := icons[ext]; ok {
		return icon
	}
	return "" // cod-file fallback
}

// RunSyncProgress runs the progress TUI, uploading each file via the upload
// callback. Returns the paths whose upload failed (empty when all succeeded).
func RunSyncProgress(header string, files []string, upload func(string) error) ([]string, error) {
	return runSyncProgress(header, files, nil, upload)
}

// RunBeamSendProgress renders the upload progress grouped by commit: one colored
// header per commit followed by its files. Uploads run in group order. Used by
// `teleport beam` so the send view mirrors the file picker's layout.
func RunBeamSendProgress(header string, groups []BeamGroup, upload func(string) error) ([]string, error) {
	files := make([]string, 0)
	for _, g := range groups {
		files = append(files, g.Paths...)
	}
	return runSyncProgress(header, files, groups, upload)
}

func runSyncProgress(header string, files []string, groups []BeamGroup, upload func(string) error) ([]string, error) {
	model := NewSyncProgress(header, len(files))
	model.groups = groups
	p := tea.NewProgram(model)

	go func() {
		for _, f := range files {
			p.Send(SyncFileDone{Path: f, Err: upload(f)})
		}
	}()

	m, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("sync progress: %w", err)
	}

	final := m.(SyncProgress)
	var failed []string
	for _, f := range final.done {
		if f.Err != nil {
			failed = append(failed, f.Path)
		}
	}
	return failed, nil
}
