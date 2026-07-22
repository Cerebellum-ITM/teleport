package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/log"
	"github.com/pascualchavez/teleport/internal/config"
	"github.com/pascualchavez/teleport/internal/git"
	"github.com/pascualchavez/teleport/internal/highlight"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/pascualchavez/teleport/internal/theme"
	"github.com/pascualchavez/teleport/internal/tui"
	"github.com/spf13/cobra"
)

var (
	mirrorBranch string
	mirrorAuto   bool
	mirrorClean  bool
	mirrorForce  bool
	mirrorYes    bool
)

var mirrorOKStyle = lipgloss.NewStyle().Foreground(theme.Success)

var mirrorCmd = &cobra.Command{
	Use:   "mirror [profile]",
	Short: "󰜘 advance the remote branch to your local commits (same hash + message)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runMirror,
}

func init() {
	mirrorCmd.Flags().StringVarP(&mirrorBranch, "branch", "b", "", "source branch (default: current branch)")
	mirrorCmd.Flags().BoolVarP(&mirrorAuto, "auto", "a", false, "advance to HEAD; skip the target picker")
	mirrorCmd.Flags().BoolVarP(&mirrorClean, "clean", "c", false, "run clean before mirroring (discard dirty remote changes)")
	mirrorCmd.Flags().BoolVarP(&mirrorForce, "force", "f", false, "allow non-fast-forward (reset --hard; discards remote commits)")
	mirrorCmd.Flags().BoolVarP(&mirrorYes, "yes", "y", false, "skip the force/clean confirmation prompt")
	registerThenFlag(mirrorCmd)
}

// mirrorResult is the --json shape for mirror.
type mirrorResult struct {
	Command     string         `json:"command"`
	Target      string         `json:"target"`
	Branch      string         `json:"branch"`
	From        string         `json:"from"`
	To          string         `json:"to"`
	Commits     []string       `json:"commits"`
	FastForward bool           `json:"fast_forward"`
	Forced      bool           `json:"forced"`
	Actions     []actionResult `json:"actions,omitempty"`
}

func runMirror(cmd *cobra.Command, args []string) error {
	profile, profileName, err := resolveProfile(args)
	if err != nil {
		return err
	}

	// Headless must be explicit about how far to advance: without a TTY the
	// target picker cannot open. -a resolves it (advance to HEAD). Fail closed
	// before connecting so nothing is transferred.
	if !interactive() && !mirrorAuto {
		return errNeedsTTY("pass -a to mirror all commits ahead of the remote")
	}

	// Validate --then actions up front so a typo (or an un-confirmable
	// confirm-action headless) fails before mirroring.
	if err := validateThenActions(profile, profileName, thenActions); err != nil {
		return err
	}
	if err := precheckActionsConfirm(profile, thenActions, mirrorYes || noInput); err != nil {
		return err
	}

	branch, err := resolveBranch(mirrorBranch)
	if err != nil {
		return err
	}
	if branch == "" {
		return nil
	}

	localTip, err := git.RevParse(branch)
	if err != nil {
		return err
	}

	client, err := connectToProfile(profile)
	if err != nil {
		return err
	}
	defer client.Close()

	dir := sshpkg.ShellQuote(profile.Path)

	// Optional clean phase (-c): reuses cleanRemote; --no-input implies -y.
	if mirrorClean {
		counts, err := cleanRemote(client, profile, mirrorYes || noInput, false)
		if err != nil {
			return err
		}
		if counts.Skipped {
			fmt.Println("aborted, no changes made")
			return nil
		}
	}

	if _, err := client.RunCommand("git -C " + dir + " rev-parse --is-inside-work-tree"); err != nil {
		return fmt.Errorf("mirror requires %s:%s to be a git working tree\nhint: cd %s && git init && git checkout -b <branch>", profile.Host, profile.Path, profile.Path)
	}

	// An unborn branch (fresh repo, no commits) makes rev-parse HEAD fail;
	// there is nothing on the remote to lose, so we bootstrap with a full
	// bundle + reset --hard.
	remoteHEAD := ""
	if out, err := client.RunCommand("git -C " + dir + " rev-parse HEAD"); err == nil {
		remoteHEAD = strings.TrimSpace(out)
	}
	unborn := remoteHEAD == ""

	// Dirty working tree guard (a reset overwrites it, so --force skips this).
	if !mirrorForce && !unborn {
		if out, err := client.RunCommand("git -C " + dir + " status --porcelain"); err == nil && strings.TrimSpace(out) != "" {
			return fmt.Errorf("remote working tree is dirty\nhint: pass -c to clean it first, or -f to force")
		}
	}

	commits, err := git.CommitsBetween(remoteHEAD, localTip)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		emit(mirrorResult{
			Command: "mirror", Target: targetOf(profile), Branch: branch,
			From: shortSHA(remoteHEAD), To: shortSHA(localTip),
			Commits: []string{}, FastForward: true,
		}, func() {
			fmt.Printf("Nothing to mirror — remote already at %s.\n", shortSHA(localTip))
		})
		return nil
	}

	// Choose the target tip (the reflected range is remoteHEAD..chosen).
	var chosen git.Commit
	if mirrorAuto {
		chosen = commits[0] // newest = branch tip
	} else {
		c, ok, err := tui.RunMirrorTargetPicker(commits, remoteHEAD, mirrorCommitDiffLoader)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("No target selected.")
			return nil
		}
		chosen = c
	}

	// Fast-forward vs force. base is what the bundle uses as prerequisite.
	base := remoteHEAD
	useReset := false
	fastForward := true
	forced := false

	if unborn {
		base = ""
		useReset = true
	} else {
		ff, err := git.IsAncestor(remoteHEAD, chosen.SHA)
		if err != nil {
			return err
		}
		if !ff {
			if !mirrorForce {
				return fmt.Errorf("remote diverged: HEAD %s is not an ancestor of %s\nhint: pass -f to overwrite (discards remote commits)", shortSHA(remoteHEAD), chosen.Short)
			}
			// --force: reset --hard is destructive, so confirm (headless needs -y).
			autoConfirm := mirrorYes || noInput
			if !autoConfirm && !interactive() {
				return errNeedsTTY("pass -y")
			}
			if !autoConfirm {
				ok, err := confirmForceMirror(profile, remoteHEAD, chosen.Short)
				if err != nil {
					return err
				}
				if !ok {
					fmt.Println("aborted, no changes made")
					return nil
				}
			}
			base, err = git.MergeBase(remoteHEAD, chosen.SHA)
			if err != nil {
				return err
			}
			useReset = true
			fastForward = false
			forced = true
		}
	}

	// Bundle the range locally and ship it to the remote's .git dir (so it is
	// invisible to `git status` and easy to remove).
	tmp, err := os.CreateTemp("", "teleport-*.bundle")
	if err != nil {
		return fmt.Errorf("create temp bundle: %w", err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	if err := git.BundleTo(base, chosen.SHA, tmp.Name()); err != nil {
		return err
	}

	remoteBundle := filepath.Join(profile.Path, ".git", "teleport-mirror.bundle")
	rbq := sshpkg.ShellQuote(remoteBundle)
	if err := client.UploadFile(tmp.Name(), remoteBundle); err != nil {
		return err
	}
	defer func() { _, _ = client.RunCommand("rm -f " + rbq) }()

	if _, err := client.RunCommand("git -C " + dir + " bundle verify " + rbq); err != nil {
		return fmt.Errorf("remote bundle verify: %w", err)
	}
	if _, err := client.RunCommand("git -C " + dir + " fetch " + rbq + " refs/teleport/mirror"); err != nil {
		return fmt.Errorf("remote fetch from bundle: %w", err)
	}
	if useReset {
		if _, err := client.RunCommand("git -C " + dir + " reset --hard FETCH_HEAD"); err != nil {
			return fmt.Errorf("remote reset --hard: %w", err)
		}
	} else {
		if _, err := client.RunCommand("git -C " + dir + " merge --ff-only FETCH_HEAD"); err != nil {
			return fmt.Errorf("remote fast-forward: %w", err)
		}
	}

	if err := config.TouchLastSync(); err != nil {
		log.Warn("could not update last sync timestamp", "err", err)
	}

	actions, actErr := executeActions(client, profile, profileName, thenActions, mirrorYes || noInput)

	res := mirrorResult{
		Command:     "mirror",
		Target:      targetOf(profile),
		Branch:      branch,
		From:        shortSHA(remoteHEAD),
		To:          chosen.Short,
		Commits:     reflectedShorts(commits, chosen.SHA),
		FastForward: fastForward,
		Forced:      forced,
		Actions:     actions,
	}
	emit(res, func() { printMirrorSummary(res) })
	return actErr
}

// mirrorCommitDiffLoader renders a commit's full diff for the target picker's
// `d` preview. All git + highlight I/O lives here so the TUI stays I/O-free
// (architecture invariant #3).
func mirrorCommitDiffLoader(sha string, width int) (tui.ViewerContent, error) {
	profile := colorprofile.Detect(os.Stdout, os.Environ())
	raw, err := git.CommitDiff(sha)
	if err != nil {
		return tui.ViewerContent{}, err
	}
	body, adds, dels := highlight.CommitDiff(raw, width, profile)
	return tui.ViewerContent{Body: body, Adds: adds, Dels: dels}, nil
}

func targetOf(p config.Profile) string { return fmt.Sprintf("%s:%s", p.Host, p.Path) }

func shortSHA(s string) string {
	if len(s) >= 7 {
		return s[:7]
	}
	return s
}

// reflectedShorts returns the short SHAs actually reflected — chosen and every
// commit older than it in the range — oldest first. commits is newest-first.
func reflectedShorts(commits []git.Commit, chosenSHA string) []string {
	idx := 0
	for i, c := range commits {
		if c.SHA == chosenSHA {
			idx = i
			break
		}
	}
	tail := commits[idx:]
	out := make([]string, 0, len(tail))
	for i := len(tail) - 1; i >= 0; i-- {
		out = append(out, tail[i].Short)
	}
	return out
}

func printMirrorSummary(res mirrorResult) {
	mode := "fast-forward"
	if res.Forced {
		mode = "forced reset"
	}
	from := res.From
	if from == "" {
		from = "∅"
	}
	fmt.Println(mirrorOKStyle.Render(fmt.Sprintf("✓ mirrored %s  %s → %s  (%d commit(s), %s)",
		res.Target, from, res.To, len(res.Commits), mode)))
}

// confirmForceMirror asks before a destructive reset --hard on the remote.
func confirmForceMirror(profile config.Profile, remoteHEAD, to string) (bool, error) {
	var ok bool
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("Force-reset %s:%s to %s?", profile.Host, profile.Path, to)).
			Description(fmt.Sprintf("Remote HEAD %s diverged — this discards remote commits and changes.", shortSHA(remoteHEAD))).
			Affirmative("Yes, force").
			Negative("Cancel").
			Value(&ok),
	))
	if err := form.Run(); err != nil {
		return false, fmt.Errorf("confirm prompt: %w", err)
	}
	return ok, nil
}
