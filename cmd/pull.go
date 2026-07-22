package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/pascualchavez/teleport/internal/git"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/pascualchavez/teleport/internal/theme"
	"github.com/spf13/cobra"
)

var pullCmd = &cobra.Command{
	Use:   "pull [profile]",
	Short: " download remote changes to local working tree",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runPull,
}

var (
	pullOKStyle     = lipgloss.NewStyle().Foreground(theme.Success)
	pullDeleteStyle = lipgloss.NewStyle().Foreground(theme.Warn)
	pullFailStyle   = lipgloss.NewStyle().Foreground(theme.Danger)
)

func init() {
	rootCmd.AddCommand(pullCmd)
}

func runPull(_ *cobra.Command, args []string) error {
	dirty, err := git.HasUncommittedChanges()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if dirty {
		return fmt.Errorf("uncommitted changes in working tree\nhint: commit or stash your changes before pulling")
	}

	profile, _, err := resolveProfile(args)
	if err != nil {
		return err
	}

	client, err := connectToProfile(profile)
	if err != nil {
		return err
	}
	defer client.Close()

	localHEAD, err := git.LocalHEAD()
	if err != nil {
		return err
	}
	remoteHEAD, err := client.RunCommand(
		"git -C " + sshpkg.ShellQuote(profile.Path) + " rev-parse HEAD",
	)
	if err != nil {
		return fmt.Errorf("get remote HEAD: %w", err)
	}
	remoteHEAD = strings.TrimSpace(remoteHEAD)
	if localHEAD != remoteHEAD {
		return fmt.Errorf(
			"remote is not at the same commit\nlocal:  %s\nremote: %s\nhint: use `git pull` to sync your commits first",
			localHEAD[:7], remoteHEAD[:7],
		)
	}

	raw, err := client.RunCommand(
		"git -C " + sshpkg.ShellQuote(profile.Path) + " status --porcelain=v1 -z",
	)
	if err != nil {
		return fmt.Errorf("remote git status: %w", err)
	}

	type entry struct {
		xy   string
		path string
	}
	var entries []entry
	for _, record := range strings.Split(raw, "\x00") {
		record = strings.TrimSpace(record)
		if len(record) < 4 {
			continue
		}
		xy := record[:2]
		path := record[3:]
		entries = append(entries, entry{xy: xy, path: path})
	}

	target := fmt.Sprintf("%s:%s", profile.Host, profile.Path)
	if len(entries) == 0 {
		emit(pullResult{Command: "pull", Target: target, Pulled: 0, Files: []string{}}, func() {
			fmt.Println("Already up to date.")
		})
		return nil
	}

	// Decoration goes to stderr under --json so stdout stays a clean object.
	out := os.Stdout
	if jsonOut {
		out = os.Stderr
	}

	var failed int
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		local := e.path
		remote := filepath.Join(profile.Path, e.path)

		x, y := string(e.xy[0]), string(e.xy[1])
		isDeleted := x == "D" || y == "D"
		if isDeleted {
			if err := os.Remove(local); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(out, "  %s %s  (%s)\n", pullFailStyle.Render("✗"), local, err)
				failed++
			} else {
				files = append(files, local)
				fmt.Fprintf(out, "  %s %s\n", pullDeleteStyle.Render("-"), local)
			}
			continue
		}

		if err := client.DownloadFile(remote, local); err != nil {
			fmt.Fprintf(out, "  %s %s  (%s)\n", pullFailStyle.Render("✗"), local, err)
			failed++
		} else {
			files = append(files, local)
			fmt.Fprintf(out, "  %s %s\n", pullOKStyle.Render("✓"), local)
		}
	}

	pulled := len(entries) - failed
	emit(pullResult{Command: "pull", Target: target, Pulled: pulled, Files: files}, func() {
		fmt.Printf("\nPulled %d file(s) from %s:%s\n", pulled, profile.Host, profile.Path)
	})

	if failed > 0 {
		return fmt.Errorf("%d file(s) failed", failed)
	}
	return nil
}

// pullResult is the --json shape for pull.
type pullResult struct {
	Command string   `json:"command"`
	Target  string   `json:"target"`
	Pulled  int      `json:"pulled"`
	Files   []string `json:"files"`
}
