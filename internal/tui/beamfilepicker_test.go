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
