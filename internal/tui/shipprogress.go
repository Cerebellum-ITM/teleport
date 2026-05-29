package tui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	lipgloss "charm.land/lipgloss/v2"
)

var (
	slActiveStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	slDoneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	slErrStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	slSkipStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	slElapsedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	slBarStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("116"))
)

var slSpinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ShipStepFunc is a step in the ship pipeline. setExtra updates the inline
// text appended to the active spinner line (e.g. upload byte progress).
type ShipStepFunc func(setExtra func(string)) error

// RunShipProgress prints all steps upfront, then animates each one in place
// using ANSI cursor movement — yellow spinner while active, green ✓ on success.
func RunShipProgress(header string, stepNames []string, steps []ShipStepFunc) ([]error, error) {
	errs := make([]error, len(steps))
	n := len(steps)

	fmt.Printf("\n  %s\n", spHeaderStyle.Render(header))

	// Print all steps as pending so the full list is visible from the start.
	for _, name := range stepNames {
		fmt.Printf("  %s  %s\n", slSkipStyle.Render("·"), slSkipStyle.Render(name))
	}

	for i, fn := range steps {
		name := stepNames[i]
		linesUp := n - i // distance from current cursor position to step i's line

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
					fmt.Printf("\033[%dA\033[2K\r  %s  %s%s\033[%dB\r",
						linesUp,
						slActiveStyle.Render(spin),
						slActiveStyle.Render(name),
						ex, // already styled by caller
						linesUp)
					frame++
				}
			}
		}()

		stepStart := time.Now()
		err := fn(setExtra)
		close(stop)
		wg.Wait()

		elapsed := time.Since(stepStart).Round(time.Millisecond)

		icon := slDoneStyle.Render("✓")
		nameStyle := slDoneStyle
		if err != nil {
			icon = slErrStyle.Render("✗")
			nameStyle = slErrStyle
			errs[i] = err
		}
		fmt.Printf("\033[%dA\033[2K\r  %s  %s  %s\033[%dB\r",
			linesUp,
			icon,
			nameStyle.Render(name),
			slElapsedStyle.Render(elapsed.String()),
			linesUp)

		if err != nil {
			return errs, nil
		}
	}

	fmt.Print("\n")
	return errs, nil
}

// TermWidth returns the terminal column count, defaulting to 120.
func TermWidth() int {
	type winsz struct{ Row, Col, Xpix, Ypix uint16 }
	ws := winsz{}
	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		os.Stdout.Fd(),
		syscall.TIOCGWINSZ,
		uintptr(unsafe.Pointer(&ws)),
	); errno == 0 && ws.Col > 0 {
		return int(ws.Col)
	}
	return 120
}

// ProgressBar returns a "[===>   ]" style bar of the given width.
func ProgressBar(written, total int64, barWidth int) string {
	if barWidth <= 0 || total <= 0 {
		return ""
	}
	pct := float64(written) / float64(total)
	filled := int(float64(barWidth) * pct)
	if filled > barWidth {
		filled = barWidth
	}
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < barWidth; i++ {
		switch {
		case i < filled-1:
			b.WriteByte('=')
		case i == filled-1 && written < total:
			b.WriteByte('>')
		case i < filled:
			b.WriteByte('=')
		default:
			b.WriteByte(' ')
		}
	}
	b.WriteByte(']')
	return slBarStyle.Render(b.String())
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
