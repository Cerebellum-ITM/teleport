package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// ShipStepDone is sent after each sequential ship step completes.
type ShipStepDone struct {
	Step int
	Err  error
}

type shipTickMsg time.Time

var (
	shStepPendStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	shStepDoneStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	shStepErrStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	shStepActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("116"))
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type ShipProgress struct {
	header  string
	steps   []string
	current int
	errs    []error
	done    []bool
	start   time.Time
	tick    int
	width   int
	height  int
}

func NewShipProgress(header string, steps []string) ShipProgress {
	return ShipProgress{
		header:  header,
		steps:   steps,
		errs:    make([]error, len(steps)),
		done:    make([]bool, len(steps)),
		current: 0,
		start:   time.Now(),
		width:   80,
		height:  24,
	}
}

func (m ShipProgress) Init() tea.Cmd {
	return shipTick()
}

func shipTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return shipTickMsg(t)
	})
}

func (m ShipProgress) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case ShipStepDone:
		m.done[msg.Step] = true
		m.errs[msg.Step] = msg.Err
		if msg.Step+1 < len(m.steps) {
			m.current = msg.Step + 1
		}
		// All steps done → quit.
		allDone := true
		for _, d := range m.done {
			if !d {
				allDone = false
				break
			}
		}
		if allDone {
			return m, tea.Quit
		}

	case shipTickMsg:
		m.tick++
		return m, shipTick()

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ShipProgress) View() tea.View {
	var b strings.Builder

	fmt.Fprintf(&b, "\n  %s\n\n", spHeaderStyle.Render(m.header))

	for i, step := range m.steps {
		switch {
		case m.done[i] && m.errs[i] == nil:
			fmt.Fprintf(&b, "  %s  %s\n",
				shStepDoneStyle.Render("✓"),
				shStepDoneStyle.Render(step))
		case m.done[i] && m.errs[i] != nil:
			fmt.Fprintf(&b, "  %s  %s\n",
				shStepErrStyle.Render("✗"),
				shStepErrStyle.Render(step))
		case i == m.current:
			frame := spinnerFrames[m.tick%len(spinnerFrames)]
			fmt.Fprintf(&b, "  %s  %s\n",
				shStepActiveStyle.Render(frame),
				shStepActiveStyle.Render(step))
		default:
			fmt.Fprintf(&b, "  %s  %s\n",
				shStepPendStyle.Render("·"),
				shStepPendStyle.Render(step))
		}
	}

	b.WriteString("\n")

	// Progress bar — treat each step as 1/N.
	doneCount := 0
	for _, d := range m.done {
		if d {
			doneCount++
		}
	}
	total := len(m.steps)
	elapsed := time.Since(m.start)
	pct := 0
	if total > 0 {
		pct = (doneCount * 100) / total
	}
	stats := fmt.Sprintf("  %d/%d  %3d%%  %s  ", doneCount, total, pct, formatSyncDuration(elapsed))
	barWidth := m.width - 4 - len(stats)
	if barWidth < 4 {
		barWidth = 4
	}
	filled := 0
	if total > 0 {
		filled = (doneCount * barWidth) / total
	}
	var inner strings.Builder
	for i := 0; i < barWidth; i++ {
		switch {
		case i < filled-1:
			inner.WriteString("=")
		case i == filled-1 && doneCount < total:
			inner.WriteString(">")
		case i == filled-1 && doneCount == total && filled > 0:
			inner.WriteString("=")
		default:
			inner.WriteString(" ")
		}
	}

	sep := spSepStyle.Render(strings.Repeat("─", m.width))
	bar := "  [" + spBarStyle.Render(inner.String()) + "]" + spStatsStyle.Render(stats)
	fmt.Fprintf(&b, "%s\n%s\n%s", sep, bar, sep)

	return tea.NewView(b.String())
}

// ShipStepFunc is a unit of work for RunShipProgress.
type ShipStepFunc func() error

// RunShipProgress runs the animated ship TUI, executing each step sequentially.
// Returns the per-step errors and any TUI error.
func RunShipProgress(header string, stepNames []string, steps []ShipStepFunc) ([]error, error) {
	model := NewShipProgress(header, stepNames)
	p := tea.NewProgram(model)

	go func() {
		for i, fn := range steps {
			err := fn()
			p.Send(ShipStepDone{Step: i, Err: err})
			if err != nil {
				// Mark remaining steps as done with no-op so TUI can quit.
				for j := i + 1; j < len(steps); j++ {
					p.Send(ShipStepDone{Step: j, Err: nil})
				}
				return
			}
		}
	}()

	m, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("ship progress: %w", err)
	}

	final := m.(ShipProgress)
	return final.errs, nil
}
