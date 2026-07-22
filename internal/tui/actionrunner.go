package tui

import (
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/pascualchavez/teleport/internal/theme"
)

var (
	arHeaderStyle = lipgloss.NewStyle().Foreground(theme.HeaderFg).Background(theme.HeaderBg).Bold(true)
	arStepStyle   = lipgloss.NewStyle().Foreground(theme.Gold)
	arOutStyle    = lipgloss.NewStyle().Foreground(theme.Text)
	arErrStyle    = lipgloss.NewStyle().Foreground(theme.Warn)
	arOKStyle     = lipgloss.NewStyle().Foreground(theme.Success)
	arFailStyle   = lipgloss.NewStyle().Foreground(theme.Danger)
	arDimStyle    = lipgloss.NewStyle().Foreground(theme.TextFaint)
)

// ActionSink renders an action's live output: a header, a styled marker per
// step, each command line streamed as it arrives, and a ✓/✗ + duration line
// when the step finishes. It streams by printing (no alternate screen) so long
// logs keep their natural terminal scrollback.
//
// A rich sink writes colored output to stdout for an interactive session; a
// plain sink writes uncolored, prefixed lines to stderr so stdout stays a clean
// --json object and headless callers still see the logs. The caller drives the
// SSH I/O and calls the methods (architecture invariant #3).
type ActionSink struct {
	name  string
	plain bool
	out   io.Writer
}

// NewActionSink builds a sink for the named action. plain selects the headless
// path (uncolored, stderr, `[name]` prefix); otherwise output is colored to
// stdout.
func NewActionSink(name string, plain bool) *ActionSink {
	out := io.Writer(os.Stdout)
	if plain {
		out = os.Stderr
	}
	return &ActionSink{name: name, plain: plain, out: out}
}

// Header prints the action banner once, before the first step.
func (s *ActionSink) Header(text string) {
	if s.plain {
		fmt.Fprintf(s.out, "[%s] %s\n", s.name, text)
		return
	}
	fmt.Fprintf(s.out, "\n  %s\n", arHeaderStyle.Render(" "+text+" "))
}

// StepStart announces step index (1-based via index/total) and its command.
func (s *ActionSink) StepStart(index, total int, label string) {
	if s.plain {
		fmt.Fprintf(s.out, "[%s] step %d/%d: %s\n", s.name, index, total, label)
		return
	}
	marker := fmt.Sprintf("step %d/%d", index, total)
	fmt.Fprintf(s.out, "\n  %s  %s\n", arStepStyle.Render("▸ "+marker), arDimStyle.Render(label))
}

// Line streams one output line from the active step. isErr tints stderr lines.
func (s *ActionSink) Line(isErr bool, text string) {
	if s.plain {
		fmt.Fprintf(s.out, "[%s] %s\n", s.name, text)
		return
	}
	style := arOutStyle
	if isErr {
		style = arErrStyle
	}
	fmt.Fprintf(s.out, "    %s\n", style.Render(text))
}

// StepDone closes the active step with a status marker and its duration. err
// non-nil marks a failure (✗); exitNonZero marks a non-zero exit that did not
// error at the transport level.
func (s *ActionSink) StepDone(err error, exitCode int, elapsed time.Duration) {
	d := elapsed.Round(time.Millisecond)
	if s.plain {
		if err != nil {
			fmt.Fprintf(s.out, "[%s] ✗ failed after %s: %v\n", s.name, d, err)
		} else if exitCode != 0 {
			fmt.Fprintf(s.out, "[%s] ✗ exit %d after %s\n", s.name, exitCode, d)
		} else {
			fmt.Fprintf(s.out, "[%s] ✓ ok in %s\n", s.name, d)
		}
		return
	}
	switch {
	case err != nil:
		fmt.Fprintf(s.out, "  %s  %s\n", arFailStyle.Render("✗"), arDimStyle.Render(fmt.Sprintf("failed after %s: %v", d, err)))
	case exitCode != 0:
		fmt.Fprintf(s.out, "  %s  %s\n", arFailStyle.Render("✗"), arDimStyle.Render(fmt.Sprintf("exit %d after %s", exitCode, d)))
	default:
		fmt.Fprintf(s.out, "  %s  %s\n", arOKStyle.Render("✓"), arDimStyle.Render("ok in "+d.String()))
	}
}

type actionConfirmModel struct {
	prompt    string
	detail    []string
	confirmed bool
	quitting  bool
}

func (m actionConfirmModel) Init() tea.Cmd { return nil }

func (m actionConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
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

func (m actionConfirmModel) View() tea.View {
	if m.confirmed || m.quitting {
		return tea.NewView("")
	}
	var b []byte
	b = append(b, []byte("\n  "+arStepStyle.Render(m.prompt)+"\n")...)
	for _, d := range m.detail {
		b = append(b, []byte("    "+arDimStyle.Render(d)+"\n")...)
	}
	b = append(b, []byte("\n"+arDimStyle.Render("  [ y / enter ] run    [ n / esc / q ] skip")+"\n")...)
	return tea.NewView(string(b))
}

// RunActionConfirm asks the user to confirm running an action, listing its
// steps as detail. Returns true on confirm, false on cancel.
func RunActionConfirm(prompt string, steps []string) (bool, error) {
	p := tea.NewProgram(actionConfirmModel{prompt: prompt, detail: steps})
	m, err := p.Run()
	if err != nil {
		return false, fmt.Errorf("action confirm: %w", err)
	}
	return m.(actionConfirmModel).confirmed, nil
}
