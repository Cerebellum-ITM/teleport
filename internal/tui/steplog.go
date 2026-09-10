package tui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// StepLog renders a command's process phases as one chronological stream:
// `scan`, `connect`, `compare`, `upload`. A phase prints its line when it starts
// and rewrites that same line as it progresses and when it ends, so a slow phase
// shows movement instead of looking hung and a fast one leaves a single line.
// Rewriting in place is safe because nothing else writes while a phase runs —
// the transfer view and the ↷ lines come between phases, not during one.
//
// It is the ActionSink shape applied to phases: colored to stdout when
// interactive, `[command]`-prefixed lines to stderr when headless so stdout stays
// a clean --json object.
type StepLog struct {
	command string
	plain   bool
	quiet   bool
	out     io.Writer

	start     time.Time
	phase     string
	phaseAt   time.Time
	live      bool
	lineLen   int
	durations map[string]time.Duration
}

func NewStepLog(command string, plain, quiet bool) *StepLog {
	out := io.Writer(os.Stdout)
	if plain {
		out = os.Stderr
	}
	return &StepLog{
		command:   command,
		plain:     plain,
		quiet:     quiet,
		out:       out,
		start:     time.Now(),
		durations: make(map[string]time.Duration),
	}
}

// Start opens a phase. detail is what is known up front, and may be empty.
func (l *StepLog) Start(phase, detail string) {
	l.phase = phase
	l.phaseAt = time.Now()
	if l.quiet {
		return
	}
	if l.plain {
		fmt.Fprintf(l.out, "[%s] %s%s\n", l.command, phase, plainDetail(detail))
		return
	}
	l.write(detail, "")
}

// Update refreshes the running phase's detail, e.g. `hashing 2/13`.
func (l *StepLog) Update(detail string) {
	if l.quiet || l.phase == "" {
		return
	}
	if l.plain {
		fmt.Fprintf(l.out, "[%s] %s%s\n", l.command, l.phase, plainDetail(detail))
		return
	}
	l.write(detail, "")
}

// Detach ends the live line but keeps the phase open, for a phase whose work is
// rendered by something else (the transfer view). Done then records the real
// duration without printing over output that has since scrolled past.
func (l *StepLog) Detach() {
	if l.quiet || l.plain || !l.live {
		return
	}
	fmt.Fprintln(l.out)
	l.live = false
}

// Done closes the running phase with its final detail and duration. After a
// Detach it only records the duration: the line it would rewrite is gone.
func (l *StepLog) Done(detail string) {
	if l.phase == "" {
		return
	}
	elapsed := time.Since(l.phaseAt)
	l.durations[l.phase] = elapsed

	if !l.quiet {
		if l.plain {
			fmt.Fprintf(l.out, "[%s] %s%s (%s)\n", l.command, l.phase, plainDetail(detail), round(elapsed))
		} else if l.live {
			l.write(detail, round(elapsed).String())
			fmt.Fprintln(l.out)
		}
	}
	l.phase = ""
	l.live = false
}

// Fail closes the running phase as failed. It names the phase and its duration
// but not the error, which the command itself reports on its way out. It prints
// even under --quiet: a silenced log must still say where the command died.
func (l *StepLog) Fail() {
	if l.phase == "" {
		return
	}
	elapsed := time.Since(l.phaseAt)
	if l.plain || l.quiet {
		fmt.Fprintf(l.out, "[%s] %s failed after %s\n", l.command, l.phase, round(elapsed))
	} else {
		l.clear()
		fmt.Fprintf(l.out, "  %s  %s\n", arFailStyle.Render("✗"), arDimStyle.Render(fmt.Sprintf("%s failed after %s", l.phase, round(elapsed))))
	}
	l.phase = ""
	l.live = false
}

// Elapsed is the time since the log was created: the command's own runtime.
func (l *StepLog) Elapsed() time.Duration { return time.Since(l.start) }

// Durations reports each finished phase's duration in seconds, for --json.
func (l *StepLog) Durations() map[string]float64 {
	out := make(map[string]float64, len(l.durations))
	for phase, d := range l.durations {
		out[phase] = d.Seconds()
	}
	return out
}

// write renders the live phase line, replacing whatever is on it.
func (l *StepLog) write(detail, elapsed string) {
	l.clear()
	line := fmt.Sprintf("  %s  %s", arStepStyle.Render("▸ "+pad(l.phase, 8)), arDimStyle.Render(detail))
	if elapsed != "" {
		line += arDimStyle.Render("  " + elapsed)
	}
	fmt.Fprint(l.out, line)
	l.live = true
	l.lineLen = len(l.phase) + len(detail) + len(elapsed) + 16
}

// clear returns the cursor to the start of the live line and blanks it, so the
// next render does not leave tails of a longer previous detail behind.
func (l *StepLog) clear() {
	if !l.live {
		return
	}
	fmt.Fprintf(l.out, "\r%s\r", strings.Repeat(" ", l.lineLen))
}

func pad(s string, width int) string {
	for len(s) < width {
		s += " "
	}
	return s
}

func plainDetail(detail string) string {
	if detail == "" {
		return ""
	}
	return ": " + detail
}

func round(d time.Duration) time.Duration {
	switch {
	case d < time.Millisecond:
		return d.Round(time.Microsecond)
	case d < time.Second:
		return d.Round(time.Millisecond)
	default:
		return d.Round(time.Second / 10)
	}
}
