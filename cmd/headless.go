package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"golang.org/x/term"
)

// ErrNeedsTTY is returned when a command would need an interactive
// selection or confirmation but is running headless (no TTY or
// --no-input). The caller maps it to exit code 2 so scripts must be
// explicit.
var ErrNeedsTTY = errors.New("requires a terminal; pass the flag explicitly to run headless")

// needsTTYError wraps ErrNeedsTTY with a hint naming the flag that would
// resolve the missing selection/confirmation from the command line.
type needsTTYError struct {
	hint string
}

func (e needsTTYError) Error() string {
	if e.hint == "" {
		return ErrNeedsTTY.Error()
	}
	return fmt.Sprintf("%s (%s)", ErrNeedsTTY.Error(), e.hint)
}

func (e needsTTYError) Unwrap() error { return ErrNeedsTTY }

// Hint returns the flag hint, e.g. "pass -a to auto-select unsent commits".
func (e needsTTYError) Hint() string { return e.hint }

// errNeedsTTY builds an ErrNeedsTTY carrying a caller-facing hint.
func errNeedsTTY(hint string) error { return needsTTYError{hint: hint} }

// interactive reports whether teleport may open a prompt/picker/viewer:
// a real TTY on stdin AND the user did not force --no-input.
func interactive() bool {
	return !noInput && term.IsTerminal(int(os.Stdin.Fd()))
}

// useTUI reports whether rich TUI progress should render: only for a
// genuinely interactive session (real TTY, no --no-input) that is not
// emitting JSON. Otherwise progress is written plainly to stderr.
func useTUI() bool { return interactive() && !jsonOut }

// emit prints v as a single JSON object to stdout when --json is set;
// otherwise it calls human to render the normal terminal output. Decorative
// output must go to stderr so --json keeps stdout a clean, parseable object.
func emit(v any, human func()) {
	if !jsonOut {
		human()
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		log.Warn("could not encode JSON result", "err", err)
	}
}

// reportError serializes err for the caller. With --json an ErrNeedsTTY is
// written to stderr as {"error":…,"hint":…}; otherwise nothing is printed
// here (cobra-style "Error:" reporting happens in Execute).
func reportError(err error) {
	if !jsonOut {
		return
	}
	payload := map[string]string{"error": ErrNeedsTTY.Error()}
	var nt needsTTYError
	if errors.As(err, &nt) && nt.hint != "" {
		payload["hint"] = nt.hint
	}
	enc := json.NewEncoder(os.Stderr)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}
