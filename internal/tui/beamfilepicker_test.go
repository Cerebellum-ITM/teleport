package tui

import (
	"strings"
	"testing"

	"github.com/pascualchavez/teleport/internal/git"
)

// stripANSI removes terminal escape sequences so View output can be matched on
// its plain text.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEsc = true
		case inEsc && r == 'm':
			inEsc = false
		case inEsc:
			// swallow the rest of the escape sequence
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func noopLoader(git.FileChange, ViewerMode, int) (ViewerContent, error) {
	return ViewerContent{}, nil
}

func TestBeamFilePickerCommitCount(t *testing.T) {
	t.Run("superseded commits surface as 'with files'", func(t *testing.T) {
		// Three commits selected, but the newest (c) reworks every path the
		// older two touched, so only c owns a winning (deduped) file.
		commits := []git.Commit{
			{SHA: "c", Short: "c"},
			{SHA: "b", Short: "b"},
			{SHA: "a", Short: "a"},
		}
		changes := []git.FileChange{
			{Path: "shared.go", Status: 'M', SHA: "c"},
		}
		m := NewBeamFilePicker(changes, commits, noopLoader)
		got := stripANSI(m.View().Content)
		if !strings.Contains(got, "3 commits · 1 with files") {
			t.Fatalf("expected gap label, got:\n%s", got)
		}
	})

	t.Run("no gap when every commit owns a file", func(t *testing.T) {
		commits := []git.Commit{
			{SHA: "b", Short: "b"},
			{SHA: "a", Short: "a"},
		}
		changes := []git.FileChange{
			{Path: "b.go", Status: 'M', SHA: "b"},
			{Path: "a.go", Status: 'A', SHA: "a"},
		}
		m := NewBeamFilePicker(changes, commits, noopLoader)
		got := stripANSI(m.View().Content)
		if !strings.Contains(got, "2 commits ") {
			t.Fatalf("expected plain count, got:\n%s", got)
		}
		if strings.Contains(got, "with files") {
			t.Fatalf("did not expect gap label, got:\n%s", got)
		}
	})
}

// pickerWith builds a picker, opens its viewer on the first file in the given
// mode, and returns it ready for navigation assertions.
func pickerWith(t *testing.T, changes []git.FileChange, commits []git.Commit, openKey string) BeamFilePicker {
	t.Helper()
	m := NewBeamFilePicker(changes, commits, noopLoader)
	next, _ := m.Update(keyPress(openKey))
	p := next.(BeamFilePicker)
	if !p.viewing {
		t.Fatalf("expected viewer open after %q", openKey)
	}
	return p
}

func TestBeamFilePickerStepFile(t *testing.T) {
	commits := []git.Commit{{SHA: "x", Short: "x"}}
	changes := []git.FileChange{
		{Path: "a.go", Status: 'M', SHA: "x"},
		{Path: "b.go", Status: 'M', SHA: "x"},
		{Path: "c.go", Status: 'M', SHA: "x"},
	}

	t.Run("→ advances and wraps, position tracks", func(t *testing.T) {
		p := pickerWith(t, changes, commits, "v")
		if p.viewer.idx != 1 || p.viewer.total != 3 {
			t.Fatalf("opened at %d/%d, want 1/3", p.viewer.idx, p.viewer.total)
		}
		want := []struct {
			path string
			idx  int
		}{{"b.go", 2}, {"c.go", 3}, {"a.go", 1}} // third → wraps to top
		for _, w := range want {
			next, _ := p.Update(keyPress("n"))
			p = next.(BeamFilePicker)
			if got := p.changes[p.cursor].Path; got != w.path {
				t.Fatalf("→ landed on %s, want %s", got, w.path)
			}
			if p.viewer.idx != w.idx {
				t.Fatalf("position %d, want %d", p.viewer.idx, w.idx)
			}
		}
	})

	t.Run("← wraps backward", func(t *testing.T) {
		p := pickerWith(t, changes, commits, "v")
		next, _ := p.Update(keyPress("p"))
		p = next.(BeamFilePicker)
		if got := p.changes[p.cursor].Path; got != "c.go" {
			t.Fatalf("← from top landed on %s, want c.go", got)
		}
	})

	t.Run("mode is preserved while paging", func(t *testing.T) {
		p := pickerWith(t, changes, commits, "d")
		if p.viewer.mode != ViewDiff {
			t.Fatalf("expected diff mode after d")
		}
		next, _ := p.Update(keyPress("n"))
		p = next.(BeamFilePicker)
		if p.viewer.mode != ViewDiff {
			t.Fatalf("mode flipped to file while paging; want diff")
		}
	})

	t.Run("single-file range is a no-op", func(t *testing.T) {
		one := []git.FileChange{{Path: "solo.go", Status: 'M', SHA: "x"}}
		p := pickerWith(t, one, commits, "v")
		next, _ := p.Update(keyPress("n"))
		p = next.(BeamFilePicker)
		if p.cursor != 0 {
			t.Fatalf("cursor moved on single-file range")
		}
	})
}
