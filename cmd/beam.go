package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/log"
	"github.com/pascualchavez/teleport/internal/config"
	"github.com/pascualchavez/teleport/internal/git"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/pascualchavez/teleport/internal/tui"
	"github.com/spf13/cobra"
)

var (
	beamBranch   string
	beamThenSync bool
	beamClean    bool
	beamYes      bool
)

var beamCmd = &cobra.Command{
	Use:   "beam [profile]",
	Short: "󰜘 send selected local commits to the remote server",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runBeam,
}

func init() {
	beamCmd.Flags().StringVarP(&beamBranch, "branch", "b", "", "source branch for commits (default: current branch)")
	beamCmd.Flags().BoolVarP(&beamThenSync, "then-sync", "s", false, "run sync after beam (working-tree changes over the just-beamed snapshot)")
	beamCmd.Flags().BoolVarP(&beamClean, "clean", "c", false, "run clean before beam (discard dirty changes on the remote)")
	beamCmd.Flags().BoolVarP(&beamYes, "yes", "y", false, "skip the clean confirmation prompt")
}

func runBeam(cmd *cobra.Command, args []string) error {
	localCfg, err := config.LoadLocal()
	if err != nil {
		return fmt.Errorf("load local config: %w", err)
	}

	profile, _, err := resolveProfile(args)
	if err != nil {
		return err
	}

	// If --clean is set, connect now and run the clean phase before
	// anything else. The connection is reused by the beam phase.
	var client *sshpkg.Client
	if beamClean {
		client, err = connectToProfile(profile)
		if err != nil {
			return err
		}
		defer func() {
			if client != nil {
				client.Close()
			}
		}()
		counts, err := cleanRemote(client, profile, beamYes, false)
		if err != nil {
			return err
		}
		if counts.Skipped {
			fmt.Println("aborted, no changes made")
			return nil
		}
	}

	branch, err := resolveBranch(beamBranch)
	if err != nil {
		return err
	}
	if branch == "" {
		return nil
	}

	commits, err := git.CommitsAheadOf(branch)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		if !beamClean {
			fmt.Printf("Nothing to beam — no local commits on %s ahead of remote.\n", branch)
		}
		return nil
	}

	selectedCommits, err := tui.RunCommitPicker(commits)
	if err != nil {
		return err
	}
	if len(selectedCommits) == 0 {
		fmt.Println("No commits selected.")
		return nil
	}

	// CommitsAhead returns newest first; FilesInCommits needs oldest first
	// so the most recent commit wins for shared paths.
	shas := make([]string, 0, len(selectedCommits))
	for i := len(selectedCommits) - 1; i >= 0; i-- {
		shas = append(shas, selectedCommits[i].SHA)
	}

	allChanges, err := git.FilesInCommits(shas)
	if err != nil {
		return err
	}
	if len(allChanges) == 0 {
		fmt.Println("Selected commits touched no files.")
		return nil
	}

	changes, err := tui.RunBeamFilePicker(allChanges, selectedCommits)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		fmt.Println("No files selected.")
		return nil
	}

	if client == nil {
		client, err = connectToProfile(profile)
		if err != nil {
			return err
		}
		defer client.Close()
	}

	var toUpload []git.FileChange
	var toDelete []git.FileChange
	for _, c := range changes {
		if c.Status == 'D' {
			toDelete = append(toDelete, c)
		} else {
			toUpload = append(toUpload, c)
		}
	}

	failures := 0

	if len(toUpload) > 0 {
		byPath := make(map[string]git.FileChange, len(toUpload))
		paths := make([]string, len(toUpload))
		for i, c := range toUpload {
			byPath[c.Path] = c
			paths[i] = c.Path
		}

		header := fmt.Sprintf("Beaming %d file(s) to %s:%s", len(paths), profile.Host, profile.Path)
		failed, err := tui.RunSyncProgress(header, paths, func(path string) error {
			fc := byPath[path]
			content, err := git.FileAtCommit(fc.SHA, fc.Path)
			if err != nil {
				return err
			}
			return client.UploadBytes(filepath.Join(profile.Path, fc.Path), content)
		})
		if err != nil {
			return err
		}
		failures += failed
	}

	for _, c := range toDelete {
		remote := filepath.Join(profile.Path, c.Path)
		if err := client.Remove(remote); err != nil {
			log.Error("remove failed", "path", remote, "err", err)
			failures++
		} else {
			log.Info("removed", "path", remote)
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d operation(s) failed", failures)
	}

	if beamThenSync {
		if err := runChainedSync(client, profile, localCfg.SyncUntracked); err != nil {
			return err
		}
	}
	if err := config.TouchLastSync(); err != nil {
		log.Warn("could not update last sync timestamp", "err", err)
	}
	return nil
}

func runChainedSync(client *sshpkg.Client, profile config.Profile, includeUntracked bool) error {
	changed, err := git.ChangedFiles()
	if err != nil {
		return fmt.Errorf("git diff: %w", err)
	}

	if includeUntracked {
		untracked, err := git.UntrackedFiles()
		if err != nil {
			log.Warn("Could not list untracked files", "err", err)
		} else {
			changed = append(changed, untracked...)
		}
	}

	changed = dedupe(changed)
	if len(changed) == 0 {
		fmt.Println("Sync stage: nothing to sync — working tree matches HEAD.")
		return nil
	}

	header := fmt.Sprintf("Syncing %d working-tree file(s) to %s:%s", len(changed), profile.Host, profile.Path)
	failed, err := tui.RunSyncProgress(header, changed, func(localPath string) error {
		return client.UploadFile(localPath, filepath.Join(profile.Path, localPath))
	})
	if err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("%d file(s) failed to upload in sync stage", failed)
	}
	return nil
}

func resolveBranch(explicit string) (string, error) {
	current, all, err := git.LocalBranches()
	if err != nil {
		return "", fmt.Errorf("list branches: %w", err)
	}
	if explicit != "" {
		for _, b := range all {
			if b == explicit {
				return b, nil
			}
		}
		return "", fmt.Errorf("branch %q not found locally", explicit)
	}
	if len(all) == 1 {
		return current, nil
	}
	return tui.RunBranchPicker(all, current)
}
