package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// CleanPlan describes the changes a clean operation is about to make
// on the remote.
type CleanPlan struct {
	Host      string
	RemoteDir string
	HeadSHA   string // short hash
	Modified  []string
	Untracked []string
	Deleted   []string
	Ignored   []string // only populated when --ignored/-x is set
}

// Total returns the number of paths the clean would touch.
func (p CleanPlan) Total() int {
	return len(p.Modified) + len(p.Untracked) + len(p.Deleted) + len(p.Ignored)
}

var (
	cleanRevertStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	cleanRemoveStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	cleanRestoreStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("116"))
	cleanIgnoredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

type cleanConfirmModel struct {
	plan      CleanPlan
	confirmed bool
	quitting  bool
}

func (m cleanConfirmModel) Init() tea.Cmd { return nil }

func (m cleanConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "y", "Y", "enter":
			m.confirmed = true
			return m, tea.Quit
		case "n", "N", "esc", "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m cleanConfirmModel) View() tea.View {
	if m.confirmed || m.quitting {
		return tea.NewView("")
	}

	var b strings.Builder

	header := fmt.Sprintf("Clean %s:%s (HEAD %s)", m.plan.Host, m.plan.RemoteDir, m.plan.HeadSHA)
	b.WriteString(headerStyle.Render("  "+header) + "\n\n")

	writeSection(&b, "Will revert", "modified", m.plan.Modified, cleanRevertStyle, iconFile)
	writeSection(&b, "Will remove", "untracked", m.plan.Untracked, cleanRemoveStyle, iconDelete)
	writeSection(&b, "Will restore", "deleted", m.plan.Deleted, cleanRestoreStyle, iconFile)
	writeSection(&b, "Will remove", "ignored", m.plan.Ignored, cleanIgnoredStyle, iconDelete)

	b.WriteString("\n" + dimStyle.Render("  [ y / enter ] confirm    [ n / esc / q ] cancel") + "\n")
	return tea.NewView(b.String())
}

func writeSection(b *strings.Builder, verb, noun string, paths []string, style lipgloss.Style, marker string) {
	if len(paths) == 0 {
		return
	}
	heading := fmt.Sprintf("  %s (%d %s):", verb, len(paths), noun)
	b.WriteString(style.Render(heading) + "\n")
	for _, p := range paths {
		b.WriteString("    " + marker + fileTypeIcon(p) + " " + p + "\n")
	}
	b.WriteString("\n")
}

// RunCleanConfirm displays the clean plan and blocks until the user
// confirms or cancels. Returns true on confirm, false on cancel.
func RunCleanConfirm(plan CleanPlan) (bool, error) {
	if plan.Total() == 0 {
		return true, nil
	}
	p := tea.NewProgram(cleanConfirmModel{plan: plan})
	m, err := p.Run()
	if err != nil {
		return false, fmt.Errorf("clean confirm: %w", err)
	}
	return m.(cleanConfirmModel).confirmed, nil
}
