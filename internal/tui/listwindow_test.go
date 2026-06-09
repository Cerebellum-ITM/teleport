package tui

import "testing"

func TestComputeWindow(t *testing.T) {
	tests := []struct {
		name                 string
		total, cursor, rows  int
		wantStart, wantEnd   int
		wantAbove, wantBelow int
	}{
		{"everything fits", 5, 2, 10, 0, 5, 0, 0},
		{"exact fit", 10, 4, 10, 0, 10, 0, 0},
		{"cursor at top", 100, 0, 12, 0, 10, 0, 90},
		{"cursor at bottom", 100, 99, 12, 90, 100, 90, 0},
		{"cursor centered", 100, 50, 12, 45, 55, 45, 45},
		{"tiny height clamps", 100, 50, 1, 50, 51, 50, 49},
		{"zero rows clamps to one", 100, 0, 0, 0, 1, 0, 99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := computeWindow(tt.total, tt.cursor, tt.rows)
			if w.start != tt.wantStart || w.end != tt.wantEnd {
				t.Errorf("range = [%d,%d), want [%d,%d)", w.start, w.end, tt.wantStart, tt.wantEnd)
			}
			if w.above != tt.wantAbove || w.below != tt.wantBelow {
				t.Errorf("hidden = (above %d, below %d), want (above %d, below %d)", w.above, w.below, tt.wantAbove, tt.wantBelow)
			}
			// The cursor must always be inside the rendered range.
			if tt.total > 0 && (tt.cursor < w.start || tt.cursor >= w.end) {
				t.Errorf("cursor %d outside window [%d,%d)", tt.cursor, w.start, w.end)
			}
		})
	}
}
