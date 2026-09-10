package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/pascualchavez/teleport/internal/theme"
)

// SyncFileDone is sent by the uploader goroutine after each file completes.
type SyncFileDone struct {
	Path string
	Err  error
}

type syncTickMsg time.Time

// syncLogRows is how many completed files the live frame shows. It is small and
// fixed on purpose: bubbletea's inline renderer owns the lines its frame covers
// and paints over whatever was there, so a frame as tall as the terminal — what
// this view used to render, blank-padded — erased the phase log above it on its
// first paint. Eight rows show the recent files and leave the log in place.
const syncLogRows = 8

// syncLogWindow is how many completed files the frame shows for a transfer of
// total files: the cap, or fewer when there is less to show, so a two-file sync
// does not render six blank rows.
func syncLogWindow(total int) int {
	if total < syncLogRows {
		return total
	}
	return syncLogRows
}

var (
	spOKStyle     = lipgloss.NewStyle().Foreground(theme.Success)
	spErrStyle    = lipgloss.NewStyle().Foreground(theme.Danger)
	spSepStyle    = lipgloss.NewStyle().Foreground(theme.TextFaint)
	spBarStyle    = lipgloss.NewStyle().Foreground(theme.Icon)
	spStatsStyle  = lipgloss.NewStyle().Foreground(theme.Text)
	spHeaderStyle = lipgloss.NewStyle().Foreground(theme.Text)
	spIconStyle   = lipgloss.NewStyle().Foreground(theme.Icon)
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

	// groups, when set, label each path with the commit it came from (beam). The
	// label is printed once, above the first file of that commit.
	groups      []BeamGroup
	groupOf     map[string]int
	headerShown map[int]bool
}

func NewSyncProgress(header string, total int) SyncProgress {
	return SyncProgress{
		header:      header,
		total:       total,
		start:       time.Now(),
		width:       80,
		headerShown: make(map[int]bool),
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

// View renders the live frame: the header, the most recent completed files, and
// the bar. Its height is fixed (see syncLogRows) so the phase log printed above
// it stays on screen.
func (m SyncProgress) View() tea.View {
	var b strings.Builder
	fmt.Fprintf(&b, "  %s\n", spHeaderStyle.Render(m.header))

	rows := syncLogWindow(m.total)
	lines := m.doneLines()
	if len(lines) > rows {
		lines = lines[len(lines)-rows:]
	}
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	for i := len(lines); i < rows; i++ {
		b.WriteString("\n")
	}

	sep := spSepStyle.Render(strings.Repeat("─", m.width))
	fmt.Fprintf(&b, "%s\n%s\n%s", sep, m.renderBar(), sep)
	return tea.NewView(b.String())
}

// doneLines renders one line per completed file, in completion order, with the
// commit's label inserted whenever the commit changes (beam). The label rides in
// the same list as the files so the window never shows a file without knowing
// which commit it came from.
func (m SyncProgress) doneLines() []string {
	var lines []string
	lastGroup := -1
	for _, f := range m.done {
		indent := "  "
		if len(m.groups) > 0 {
			indent = "      "
			if i, ok := m.groupOf[f.Path]; ok && i != lastGroup {
				lastGroup = i
				g := m.groups[i]
				lines = append(lines, "  "+g.Style.Render(iconCube+"["+g.Short+"]")+" "+
					spSepStyle.Render("─")+" "+spHeaderStyle.Render(g.Subject))
			}
		}

		icon := spIconStyle.Render(fileTypeIcon(f.Path))
		if f.Err != nil {
			lines = append(lines, fmt.Sprintf("%s%s %s %s", indent, spErrStyle.Render("✗"), icon, spErrStyle.Render(f.Path)))
		} else {
			lines = append(lines, fmt.Sprintf("%s%s %s %s", indent, spOKStyle.Render("✓"), icon, f.Path))
		}
	}
	return lines
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

// RunSyncPlain uploads each file without opening a TUI, writing one progress
// line per file to stderr so stdout stays clean for --json. Used in headless
// mode (no TTY or --no-input) where the bubbletea program must not run.
func RunSyncPlain(header string, files []string, upload func(string) error) ([]string, error) {
	return runPlain(header, files, upload)
}

// RunBeamSendPlain is the headless counterpart of RunBeamSendProgress: it
// uploads every file across all groups in order, without a TUI.
func RunBeamSendPlain(header string, groups []BeamGroup, upload func(string) error) ([]string, error) {
	files := make([]string, 0)
	for _, g := range groups {
		files = append(files, g.Paths...)
	}
	return runPlain(header, files, upload)
}

func runPlain(header string, files []string, upload func(string) error) ([]string, error) {
	fmt.Fprintf(os.Stderr, "%s\n", header)
	var failed []string
	for _, f := range files {
		if err := upload(f); err != nil {
			failed = append(failed, f)
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", f, err)
		} else {
			fmt.Fprintf(os.Stderr, "  ✓ %s\n", f)
		}
	}
	return failed, nil
}

func runSyncProgress(header string, files []string, groups []BeamGroup, upload func(string) error) ([]string, error) {
	model := NewSyncProgress(header, len(files))
	model.groups = groups
	model.groupOf = make(map[string]int, len(files))
	for i, g := range groups {
		for _, path := range g.Paths {
			model.groupOf[path] = i
		}
	}

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
