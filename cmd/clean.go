package cmd

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/log"
	"github.com/pascualchavez/teleport/internal/config"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/pascualchavez/teleport/internal/tui"
	"github.com/spf13/cobra"
)

var (
	cleanYes     bool
	cleanIgnored bool
)

var cleanCmd = &cobra.Command{
	Use:   "clean [profile]",
	Short: "󰃢 discard dirty changes on the remote (git checkout + git clean)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runClean,
}

func init() {
	cleanCmd.Flags().BoolVarP(&cleanYes, "yes", "y", false,
		"skip the confirmation prompt")
	cleanCmd.Flags().BoolVarP(&cleanIgnored, "ignored", "x", false,
		"also remove gitignored files (builds, caches, node_modules)")
}

var cleanOKStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))

type cleanCounts struct {
	Reverted       int
	Removed        int
	Restored       int
	RemovedIgnored int
	Skipped        bool // user cancelled the prompt
}

func runClean(cmd *cobra.Command, args []string) error {
	profile, _, err := resolveProfile(args)
	if err != nil {
		return err
	}

	client, err := connectToProfile(profile)
	if err != nil {
		return err
	}
	defer client.Close()

	counts, err := cleanRemote(client, profile, cleanYes, cleanIgnored)
	if err != nil {
		return err
	}
	if counts.Skipped {
		fmt.Println("aborted, no changes made")
		return nil
	}
	if counts.Reverted+counts.Removed+counts.Restored+counts.RemovedIgnored == 0 {
		// already-clean message printed inside cleanRemote
		return nil
	}

	fmt.Println(cleanOKStyle.Render(fmt.Sprintf("✓ cleaned %s:%s", profile.Host, profile.Path)))
	if counts.Reverted > 0 {
		fmt.Printf("  reverted: %d file(s)\n", counts.Reverted)
	}
	if counts.Removed > 0 {
		fmt.Printf("  removed:  %d file(s)\n", counts.Removed)
	}
	if counts.Restored > 0 {
		fmt.Printf("  restored: %d file(s)\n", counts.Restored)
	}
	if counts.RemovedIgnored > 0 {
		fmt.Printf("  removed (ignored): %d file(s)\n", counts.RemovedIgnored)
	}
	return nil
}

// resolveProfile loads local + global config and returns the resolved profile.
func resolveProfile(args []string) (config.Profile, string, error) {
	localCfg, err := config.LoadLocal()
	if err != nil {
		return config.Profile{}, "", fmt.Errorf("load local config: %w", err)
	}

	name := localCfg.DefaultProfile
	if len(args) > 0 {
		name = args[0]
	}
	if name == "" {
		return config.Profile{}, "", fmt.Errorf("no profile specified; run `teleport init` first or pass a profile name")
	}

	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return config.Profile{}, "", fmt.Errorf("load global config: %w", err)
	}
	profile, ok := globalCfg.Profiles[name]
	if !ok {
		return config.Profile{}, "", fmt.Errorf("profile %q not found; run `teleport init` to create it", name)
	}
	return profile, name, nil
}

// connectToProfile parses ~/.ssh/config, resolves the host, and connects.
func connectToProfile(profile config.Profile) (*sshpkg.Client, error) {
	hosts, err := sshpkg.ParseSSHConfig()
	if err != nil {
		return nil, fmt.Errorf("parse ssh config: %w", err)
	}
	var target *sshpkg.Host
	for _, h := range hosts {
		if h.Name == profile.Host {
			hCopy := h
			target = &hCopy
			break
		}
	}
	if target == nil {
		target = &sshpkg.Host{
			Name:     profile.Host,
			Hostname: profile.Host,
			Port:     "22",
		}
	}
	log.Info("Connecting", "host", target.Name)
	client, err := sshpkg.Connect(*target)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", target.Name, err)
	}
	return client, nil
}

// cleanRemote runs the safe-guard check, the porcelain status, the
// confirmation TUI (unless assumeYes), and finally `git checkout -- .` +
// `git clean -fd` on the remote.
func cleanRemote(client *sshpkg.Client, profile config.Profile, assumeYes, includeIgnored bool) (cleanCounts, error) {
	dir := sshpkg.ShellQuote(profile.Path)

	if _, err := client.RunCommand("git -C " + dir + " rev-parse --is-inside-work-tree"); err != nil {
		return cleanCounts{}, fmt.Errorf("clean requires %s:%s to be a git working tree\nhint: cd %s && git init && git remote add ... && git fetch", profile.Host, profile.Path, profile.Path)
	}

	headOut, err := client.RunCommand("git -C " + dir + " rev-parse --short HEAD")
	if err != nil {
		return cleanCounts{}, fmt.Errorf("read remote HEAD: %w", err)
	}
	head := strings.TrimSpace(headOut)

	statusCmd := "git -C " + dir + " status --porcelain=v1 -z"
	if includeIgnored {
		statusCmd += " --ignored"
	}
	statusOut, err := client.RunCommand(statusCmd)
	if err != nil {
		return cleanCounts{}, fmt.Errorf("read remote status: %w", err)
	}

	modified, untracked, deleted, ignored := parsePorcelain(statusOut)

	total := len(modified) + len(untracked) + len(deleted) + len(ignored)
	if total == 0 {
		fmt.Println(cleanOKStyle.Render(fmt.Sprintf("✓ %s:%s already clean (HEAD %s)", profile.Host, profile.Path, head)))
		return cleanCounts{}, nil
	}

	if !assumeYes {
		plan := tui.CleanPlan{
			Host:      profile.Host,
			RemoteDir: profile.Path,
			HeadSHA:   head,
			Modified:  modified,
			Untracked: untracked,
			Deleted:   deleted,
			Ignored:   ignored,
		}
		ok, err := tui.RunCleanConfirm(plan)
		if err != nil {
			return cleanCounts{}, err
		}
		if !ok {
			return cleanCounts{Skipped: true}, nil
		}
	}

	if _, err := client.RunCommand("git -C " + dir + " checkout -- ."); err != nil {
		return cleanCounts{}, fmt.Errorf("git checkout on remote: %w", err)
	}
	cleanCmd := "git -C " + dir + " clean -fd"
	if includeIgnored {
		cleanCmd += " -x"
	}
	if _, err := client.RunCommand(cleanCmd); err != nil {
		return cleanCounts{}, fmt.Errorf("git clean on remote: %w", err)
	}

	return cleanCounts{
		Reverted:       len(modified),
		Removed:        len(untracked),
		Restored:       len(deleted),
		RemovedIgnored: len(ignored),
	}, nil
}

// parsePorcelain parses the output of `git status --porcelain=v1 -z`
// into four categories: modified (revert), untracked (remove),
// deleted-in-worktree (restore), and ignored (remove with -x).
//
// porcelain v1 records are NUL-terminated. Each record starts with 2
// status chars + space + path. Rename/copy records contain a second
// NUL-separated path; we keep the new (post-rename) path.
func parsePorcelain(out string) (modified, untracked, deleted, ignored []string) {
	if out == "" {
		return
	}
	records := strings.Split(out, "\x00")
	// trailing NUL produces an empty final element
	for i := 0; i < len(records); i++ {
		r := records[i]
		if len(r) < 3 {
			continue
		}
		xy := r[:2]
		path := r[3:]

		// rename/copy carries the source path in the next record
		if xy[0] == 'R' || xy[0] == 'C' {
			// next entry is the original path; current `path` is the new path
			if i+1 < len(records) {
				i++ // skip the source path
			}
		}

		switch {
		case xy == "??":
			untracked = append(untracked, path)
		case xy == "!!":
			ignored = append(ignored, path)
		case xy[1] == 'D':
			deleted = append(deleted, path)
		default:
			modified = append(modified, path)
		}
	}
	return
}
