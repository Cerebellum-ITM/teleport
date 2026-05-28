package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	lipgloss "charm.land/lipgloss/v2"
)

var (
	slActiveStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	slDoneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	slErrStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	slSkipStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	slElapsedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

var slSpinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ShipStepFunc is a step in the ship pipeline. setExtra updates the inline
// text appended to the active spinner line (e.g. upload byte progress).
type ShipStepFunc func(setExtra func(string)) error

// RunShipProgress executes each step sequentially, printing log-style spinner
// lines — yellow while active, green on success, red on error.
// The second return value is always nil (kept for API compatibility).
func RunShipProgress(header string, stepNames []string, steps []ShipStepFunc) ([]error, error) {
	errs := make([]error, len(steps))

	fmt.Printf("\n  %s\n\n", spHeaderStyle.Render(header))

	for i, fn := range steps {
		name := stepNames[i]

		var mu sync.Mutex
		extra := ""
		setExtra := func(s string) {
			mu.Lock()
			extra = s
			mu.Unlock()
		}

		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(80 * time.Millisecond)
			defer ticker.Stop()
			frame := 0
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					mu.Lock()
					ex := extra
					mu.Unlock()
					spin := slSpinFrames[frame%len(slSpinFrames)]
					fmt.Printf("\r  %s  %-22s%s",
						slActiveStyle.Render(spin),
						slActiveStyle.Render(name),
						slActiveStyle.Render(ex))
					frame++
				}
			}
		}()

		stepStart := time.Now()
		err := fn(setExtra)
		close(stop)
		wg.Wait()

		elapsed := time.Since(stepStart).Round(time.Millisecond)
		fmt.Printf("\r%s\r", strings.Repeat(" ", 100))

		if err != nil {
			fmt.Printf("  %s  %-22s%s\n",
				slErrStyle.Render("✗"),
				slErrStyle.Render(name),
				slElapsedStyle.Render(elapsed.String()))
			errs[i] = err
			for j := i + 1; j < len(steps); j++ {
				fmt.Printf("  %s  %s\n",
					slSkipStyle.Render("·"),
					slSkipStyle.Render(stepNames[j]))
			}
			return errs, nil
		}

		fmt.Printf("  %s  %-22s%s\n",
			slDoneStyle.Render("✓"),
			slDoneStyle.Render(name),
			slElapsedStyle.Render(elapsed.String()))
	}

	return errs, nil
}

// HumanBytes formats a byte count as a human-readable string.
func HumanBytes(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
