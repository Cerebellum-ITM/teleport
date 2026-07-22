package tui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

// captureSink swaps a plain sink's writer for a buffer so the headless output
// (which the streaming SSH path emits) can be asserted deterministically.
func captureSink(name string) (*ActionSink, *bytes.Buffer) {
	s := NewActionSink(name, true)
	buf := &bytes.Buffer{}
	s.out = buf
	return s, buf
}

func TestActionSinkPlainStream(t *testing.T) {
	s, buf := captureSink("deploy")
	s.Header("deploy · h:/srv")
	s.StepStart(1, 2, "echo build")
	s.Line(false, "building...")
	s.Line(true, "warning: deprecated")
	s.StepDone(nil, 0, 1500*time.Millisecond)

	out := buf.String()
	for _, want := range []string{
		"[deploy] deploy · h:/srv",
		"[deploy] step 1/2: echo build",
		"[deploy] building...",
		"[deploy] warning: deprecated",
		"[deploy] ✓ ok in 1.5s",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("plain output missing %q\n---\n%s", want, out)
		}
	}
}

func TestActionSinkStepDoneStates(t *testing.T) {
	t.Run("non-zero exit", func(t *testing.T) {
		s, buf := captureSink("a")
		s.StepDone(nil, 3, 200*time.Millisecond)
		if !strings.Contains(buf.String(), "✗ exit 3") {
			t.Fatalf("want exit-3 marker, got %q", buf.String())
		}
	})
	t.Run("transport error", func(t *testing.T) {
		s, buf := captureSink("a")
		s.StepDone(errors.New("boom"), -1, 200*time.Millisecond)
		if !strings.Contains(buf.String(), "✗ failed") || !strings.Contains(buf.String(), "boom") {
			t.Fatalf("want failure marker with cause, got %q", buf.String())
		}
	})
}
