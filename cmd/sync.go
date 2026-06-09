package cmd

import (
	"fmt"
	"path/filepath"

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

	changed, err := git.ChangedFiles()
	if err != nil {
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
		fmt.Println("Nothing to sync — no changes since last commit.")
		return nil
	}

	client, err := connectToProfile(profile)
	if err != nil {
		return err
	}
	defer client.Close()

	header := fmt.Sprintf("Syncing %d file(s) to %s:%s", len(changed), profile.Host, profile.Path)
	failed, err := tui.RunSyncProgress(header, changed, func(localPath string) error {
		return client.UploadFile(localPath, filepath.Join(profile.Path, localPath))
	})
	if err != nil {
		return err
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d file(s) failed to upload", len(failed))
	}
	if skippedUntracked > 0 {
		log.Warn(
			fmt.Sprintf("%d untracked file(s) not included", skippedUntracked),
			"hint", "use -u, or `teleport config set sync-untracked true`",
		)
	}
	if err := config.TouchLastSync(); err != nil {
		log.Warn("could not update last sync timestamp", "err", err)
	}
	return nil
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
