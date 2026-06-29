package tui

import (
	"sort"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/pascualchavez/teleport/internal/git"
)

// keyPress builds a KeyPressMsg whose String() resolves to key (its Text), which
// is what CommitPicker.Update switches on. Sufficient for printable keys (m, M).
func keyPress(key string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: key}
}

func sortStrings(s []string) []string { sort.Strings(s); return s }

func commitsFor(shas ...string) []git.Commit {
	out := make([]git.Commit, 0, len(shas))
	for _, s := range shas {
		out = append(out, git.Commit{SHA: s, Short: s})
	}
	return out
}

func TestCommitPickerSentDelta(t *testing.T) {
	t.Run("individual toggle marks and unmarks", func(t *testing.T) {
		// a starts unsent, b starts sent.
		p := NewCommitPicker(commitsFor("a", "b"), map[string]bool{"b": true})
		// Mark a sent (cursor at 0), then unmark b.
		p.cursor = 0
		p, _ = applyKey(p, "m")
		p.cursor = 1
		p, _ = applyKey(p, "m")

		d := p.SentDelta()
		if got := sortStrings(d.Added); len(got) != 1 || got[0] != "a" {
			t.Fatalf("Added = %v, want [a]", got)
		}
		if got := sortStrings(d.Removed); len(got) != 1 || got[0] != "b" {
			t.Fatalf("Removed = %v, want [b]", got)
		}
	})

	t.Run("toggling back to original yields no delta", func(t *testing.T) {
		p := NewCommitPicker(commitsFor("a"), nil)
		p.cursor = 0
		p, _ = applyKey(p, "m") // mark
		p, _ = applyKey(p, "m") // unmark — back to original (unsent)
		d := p.SentDelta()
		if len(d.Added) != 0 || len(d.Removed) != 0 {
			t.Fatalf("delta = %+v, want empty", d)
		}
	})

	t.Run("bulk M marks all then unmarks all (symmetric)", func(t *testing.T) {
		p := NewCommitPicker(commitsFor("a", "b", "c"), map[string]bool{"b": true})
		// Not all sent → M marks all. a and c become sent (b already was).
		p, _ = applyKey(p, "M")
		d := p.SentDelta()
		if got := sortStrings(d.Added); len(got) != 2 || got[0] != "a" || got[1] != "c" {
			t.Fatalf("Added = %v, want [a c]", got)
		}
		if len(d.Removed) != 0 {
			t.Fatalf("Removed = %v, want none", d.Removed)
		}

		// Now all sent → M unmarks all. Only b was originally sent → only b removed.
		p, _ = applyKey(p, "M")
		d = p.SentDelta()
		if len(d.Added) != 0 {
			t.Fatalf("Added = %v, want none", d.Added)
		}
		if got := sortStrings(d.Removed); len(got) != 1 || got[0] != "b" {
			t.Fatalf("Removed = %v, want [b]", got)
		}
	})

	t.Run("delta never reports a SHA outside the shown commits", func(t *testing.T) {
		// History has a stale SHA "z" not among the picker's commits. Unmarking
		// everything must not report "z" as removed — it was never shown.
		p := NewCommitPicker(commitsFor("a"), map[string]bool{"a": true, "z": true})
		p.cursor = 0
		p, _ = applyKey(p, "m") // unmark a
		d := p.SentDelta()
		if got := sortStrings(d.Removed); len(got) != 1 || got[0] != "a" {
			t.Fatalf("Removed = %v, want [a] only (never z)", got)
		}
		for _, s := range d.Removed {
			if s == "z" {
				t.Fatal("z reported as removed but it was not a shown commit")
			}
		}
	})
}

// applyKey feeds a single key string through the picker's Update and returns the
// resulting model.
func applyKey(p CommitPicker, key string) (CommitPicker, error) {
	m, _ := p.Update(keyPress(key))
	return m.(CommitPicker), nil
}
