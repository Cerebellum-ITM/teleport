package cmd

import (
	"sort"
	"testing"
)

func TestCommitsFullySent(t *testing.T) {
	sortS := func(s []string) []string { sort.Strings(s); return s }

	t.Run("shared file credits both commits", func(t *testing.T) {
		// A (old) and B (new) both touched X; B won the blob but X was sent, so
		// BOTH commits must be marked — this is the bug being fixed.
		commitPaths := map[string][]string{
			"A": {"X"},
			"B": {"X"},
		}
		covered := map[string]bool{"X": true}
		got := sortS(commitsFullySent(commitPaths, covered))
		want := []string{"A", "B"}
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("a commit with an uncovered path is not sent", func(t *testing.T) {
		// A touched X and Y; Y was deselected/failed (not covered) → A not sent.
		commitPaths := map[string][]string{
			"A": {"X", "Y"},
			"B": {"X"},
		}
		covered := map[string]bool{"X": true}
		got := commitsFullySent(commitPaths, covered)
		if len(got) != 1 || got[0] != "B" {
			t.Fatalf("got %v, want [B]", got)
		}
	})

	t.Run("failed path uncovers its commit", func(t *testing.T) {
		commitPaths := map[string][]string{"A": {"X"}}
		covered := map[string]bool{} // X failed → not covered
		if got := commitsFullySent(commitPaths, covered); len(got) != 0 {
			t.Fatalf("got %v, want none", got)
		}
	})

	t.Run("commit with no paths is skipped", func(t *testing.T) {
		commitPaths := map[string][]string{"A": {}}
		if got := commitsFullySent(commitPaths, map[string]bool{}); len(got) != 0 {
			t.Fatalf("got %v, want none", got)
		}
	})
}
