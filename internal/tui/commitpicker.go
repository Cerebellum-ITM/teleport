package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/pascualchavez/teleport/internal/git"
)

const (
	iconCommit = "󰜘 "
	iconSent   = "󰗠 "
)

type CommitPicker struct {
	commits  []git.Commit
	selected map[int]bool
	sent     map[string]bool // commit SHA → already beamed to the active profile (live, mutated by m/M)
	origSent map[string]bool // snapshot of sent at construction, for delta computation
	cursor   int
	height   int
	done     bool
	quitting bool
}

// SentMarkDelta reports the manual sent-mark changes made during a commit
// picker session: Added are SHAs newly marked sent, Removed are SHAs unmarked.
type SentMarkDelta struct {
	Added   []string
	Removed []string
}

var (
	commitShortStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	commitDateStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	sentStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
)

func NewCommitPicker(commits []git.Commit, sent map[string]bool) CommitPicker {
	if sent == nil {
		sent = map[string]bool{}
	}
	selected := make(map[int]bool, len(commits))
	for i, c := range commits {
		// Pre-select exactly the commits not yet beamed to this profile.
		selected[i] = !sent[c.SHA]
	}
	// Snapshot the original sent set so SentDelta can diff against it after the
	// user mutates the live `sent` map with m/M.
	origSent := make(map[string]bool, len(sent))
	for sha := range sent {
		origSent[sha] = true
	}
	return CommitPicker{
		commits:  commits,
		selected: selected,
		sent:     sent,
		origSent: origSent,
		height:   24,
	}
}

func (m CommitPicker) Init() tea.Cmd { return nil }

func (m CommitPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.cursor < len(m.commits)-1 {
				m.cursor++
			}
		case "tab", " ":
			if len(m.commits) > 0 {
				m.selected[m.cursor] = !m.selected[m.cursor]
			}
		case "a":
			allOn := true
			for i := range m.commits {
				if !m.selected[i] {
					allOn = false
					break
				}
			}
			for i := range m.commits {
				m.selected[i] = !allOn
			}
		case "u":
			// Re-select exactly the commits not yet beamed to this profile.
			for i, c := range m.commits {
				m.selected[i] = !m.sent[c.SHA]
			}
		case "m":
			// Toggle the manual sent-mark of the commit under the cursor (live
			// dim/badge update). Independent of the beam selection.
			if len(m.commits) > 0 {
				sha := m.commits[m.cursor].SHA
				if m.sent[sha] {
					delete(m.sent, sha)
				} else {
					m.sent[sha] = true
				}
			}
		case "M":
			// Bulk, symmetric: if every commit is already marked sent, unmark
			// them all; otherwise mark them all sent.
			allSent := true
			for _, c := range m.commits {
				if !m.sent[c.SHA] {
					allSent = false
					break
				}
			}
			for _, c := range m.commits {
				if allSent {
					delete(m.sent, c.SHA)
				} else {
					m.sent[c.SHA] = true
				}
			}
		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m CommitPicker) View() tea.View {
	if m.quitting || m.done {
		return tea.NewView("")
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("  Local commits ahead of upstream") + "\n\n")

	// chrome: header(1) + blank(1) + blank(1) + footer(1) = 4 lines.
	win := computeWindow(len(m.commits), m.cursor, m.height-4)
	if h := scrollUpHint(win.above); h != "" {
		b.WriteString(h + "\n")
	}
	for i := win.start; i < win.end; i++ {
		c := m.commits[i]
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

		// Sent badge: green icon for beamed commits, blank (same width) for the
		// rest so the columns stay aligned. The subject is dimmed when sent.
		subject := c.Subject
		badge := strings.Repeat(" ", len([]rune(iconSent)))
		if m.sent[c.SHA] {
			badge = sentStyle.Render(iconSent)
			subject = dimStyle.Render(subject)
		}

		line := fmt.Sprintf("%s%s%s%s  %s  %s",
			prefix,
			mark,
			badge,
			commitShortStyle.Render(c.Short),
			subject,
			commitDateStyle.Render(c.RelDate),
		)
		b.WriteString(line + "\n")
	}
	if h := scrollDownHint(win.below); h != "" {
		b.WriteString(h + "\n")
	}

	b.WriteString("\n" + dimStyle.Render("  tab=toggle  a=all  u=unsent  m=mark-sent  M=all-sent  enter=confirm  ctrl+c=quit") + "\n")
	return tea.NewView(b.String())
}

func (m CommitPicker) SelectedCommits() []git.Commit {
	out := make([]git.Commit, 0, len(m.selected))
	for i, c := range m.commits {
		if m.selected[i] {
			out = append(out, c)
		}
	}
	return out
}

// SentDelta reports the manual sent-mark changes made during this session,
// scoped to the commits actually shown. It iterates only over m.commits, so a
// SHA present in the beamed store but outside the displayed commits-ahead
// window can never be reported as removed.
func (m CommitPicker) SentDelta() SentMarkDelta {
	var d SentMarkDelta
	for _, c := range m.commits {
		now, orig := m.sent[c.SHA], m.origSent[c.SHA]
		switch {
		case now && !orig:
			d.Added = append(d.Added, c.SHA)
		case !now && orig:
			d.Removed = append(d.Removed, c.SHA)
		}
	}
	return d
}

func RunCommitPicker(commits []git.Commit, sent map[string]bool) ([]git.Commit, SentMarkDelta, error) {
	if len(commits) == 0 {
		return nil, SentMarkDelta{}, nil
	}
	p := tea.NewProgram(NewCommitPicker(commits, sent))
	m, err := p.Run()
	if err != nil {
		return nil, SentMarkDelta{}, fmt.Errorf("commit picker: %w", err)
	}
	result := m.(CommitPicker)
	if result.quitting {
		return nil, SentMarkDelta{}, fmt.Errorf("cancelled")
	}
	return result.SelectedCommits(), result.SentDelta(), nil
}
