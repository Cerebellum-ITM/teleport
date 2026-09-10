package cmd

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/charmbracelet/log"
	"github.com/pascualchavez/teleport/internal/config"
	"github.com/pascualchavez/teleport/internal/git"
	"github.com/pascualchavez/teleport/internal/tui"
	"github.com/spf13/cobra"
)

var includeUntracked bool

var syncCmd = &cobra.Command{
	Use:   "sync [profile]",
	Short: " sync changed files to the remote server",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSync,
}

func init() {
	syncCmd.Flags().BoolVarP(&includeUntracked, "untracked", "u", false, "also sync untracked files")
	registerThenFlag(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	localCfg, err := config.LoadLocal()
	if err != nil {
		return fmt.Errorf("load local config: %w", err)
	}

	profileName := localCfg.DefaultProfile
	if len(args) > 0 {
		profileName = args[0]
	}
	if profileName == "" {
		return fmt.Errorf("no profile specified; run `teleport init` first or pass a profile name")
	}

	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("load global config: %w", err)
	}

	profile, ok := globalCfg.Profiles[profileName]
	if !ok {
		return fmt.Errorf("profile %q not found; run `teleport init` to create it", profileName)
	}

	steps := newStepLog("sync")

	steps.Start("scan", "git diff HEAD")
	changed, err := git.ChangedFiles()
	if err != nil {
		steps.Fail()
		return fmt.Errorf("git diff: %w", err)
	}

	effectiveUntracked := includeUntracked || localCfg.SyncUntracked
	var skippedUntracked int

	if effectiveUntracked {
		untracked, err := git.UntrackedFiles()
		if err != nil {
			log.Warn("Could not list untracked files", "err", err)
		} else {
			changed = append(changed, untracked...)
		}
	} else {
		untracked, err := git.UntrackedFiles()
		if err == nil {
			skippedUntracked = len(untracked)
		}
	}

	changed = dedupe(changed)

	if len(changed) == 0 {
		steps.Done("no changes")
		fmt.Println("Nothing to sync — no changes since last commit.")
		return nil
	}
	scanned := fmt.Sprintf("%d file(s)", len(changed))
	if skippedUntracked > 0 {
		scanned += fmt.Sprintf(" · %d untracked left out", skippedUntracked)
	}
	steps.Done(scanned)

	// Validate --then actions before uploading so a typo (or an un-confirmable
	// confirm-action headless) fails fast.
	if err := validateThenActions(profile, profileName, thenActions); err != nil {
		return err
	}
	if err := precheckActionsConfirm(profile, thenActions, noInput); err != nil {
		return err
	}

	target := fmt.Sprintf("%s:%s", profile.Host, profile.Path)

	steps.Start("connect", target)
	client, err := connectToProfile(profile)
	if err != nil {
		steps.Fail()
		return err
	}
	defer client.Close()
	steps.Done("")

	steps.Start("upload", fmt.Sprintf("%d file(s)", len(changed)))
	steps.Detach()

	header := fmt.Sprintf("Syncing %d file(s) to %s", len(changed), target)
	upload := func(localPath string) error {
		return client.UploadFile(localPath, filepath.Join(profile.Path, localPath))
	}
	var failed []string
	if useTUI() {
		failed, err = tui.RunSyncProgress(header, changed, upload)
	} else {
		failed, err = tui.RunSyncPlain(header, changed, upload)
	}
	if err != nil {
		return err
	}
	if len(failed) > 0 {
		steps.Fail()
		return fmt.Errorf("%d file(s) failed to upload", len(failed))
	}
	steps.Done("")
	if skippedUntracked > 0 {
		log.Warn(
			fmt.Sprintf("%d untracked file(s) not included", skippedUntracked),
			"hint", "use -u, or `teleport config set sync-untracked true`",
		)
	}
	if err := config.TouchLastSync(); err != nil {
		log.Warn("could not update last sync timestamp", "err", err)
	}

	actions, actErr := executeActions(client, profile, profileName, thenActions, noInput)

	res := syncResult{
		Command: "sync",
		Target:  target,
		Sent:    len(changed),
		Phases:  steps.Durations(),
		Files:   changed,
		Actions: actions,
	}
	emit(res, func() {
		fmt.Printf("  %s synced %d file(s) → %s%s\n",
			okMark, len(changed), target, elapsedStyle.Render("  "+steps.Elapsed().Round(time.Millisecond).String()))
	})
	return actErr
}

// syncResult is the --json shape for sync.
type syncResult struct {
	Command string             `json:"command"`
	Target  string             `json:"target"`
	Sent    int                `json:"sent"`
	Phases  map[string]float64 `json:"phases,omitempty"`
	Files   []string           `json:"files"`
	Actions []actionResult     `json:"actions,omitempty"`
}

func dedupe(files []string) []string {
	seen := make(map[string]struct{}, len(files))
	out := make([]string, 0, len(files))
	for _, f := range files {
		if _, ok := seen[f]; !ok {
			seen[f] = struct{}{}
			out = append(out, f)
		}
	}
	return out
}
