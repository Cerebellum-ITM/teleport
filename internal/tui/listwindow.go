package tui

import "fmt"

// listWindow describes which slice of a list to render so it never overflows
// the terminal height. Pickers compute it from the total item count, the
// cursor position, and the number of rows available for the list.
type listWindow struct {
	start, end   int // [start, end) range of items to render
	above, below int // counts hidden above / below the visible window
}

// computeWindow picks the slice of `total` items to show within `rows` lines,
// keeping `cursor` visible and roughly centered. When the list doesn't fully
// fit, two rows are reserved for the scroll hints. When everything fits, the
// full range is returned with no hidden counts.
func computeWindow(total, cursor, rows int) listWindow {
	if rows < 1 {
		rows = 1
	}
	if total <= rows {
		return listWindow{start: 0, end: total}
	}

	// Reserve two rows for the "↑ N more" / "↓ N more" hints.
	itemRows := rows - 2
	if itemRows < 1 {
		itemRows = 1
	}

	start := cursor - itemRows/2
	if start < 0 {
		start = 0
	}
	end := start + itemRows
	if end > total {
		end = total
		start = end - itemRows
	}
	if start < 0 {
		start = 0
	}
	return listWindow{start: start, end: end, above: start, below: total - end}
}

// scrollUpHint renders the "items hidden above" indicator, or "" when none.
func scrollUpHint(n int) string {
	if n <= 0 {
		return ""
	}
	return dimStyle.Render(fmt.Sprintf("  ↑ %d more", n))
}

// scrollDownHint renders the "items hidden below" indicator, or "" when none.
func scrollDownHint(n int) string {
	if n <= 0 {
		return ""
	}
	return dimStyle.Render(fmt.Sprintf("  ↓ %d more", n))
}

// truncate shortens s to at most n runes, appending an ellipsis when cut.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}
