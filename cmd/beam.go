package cmd

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/log"
	"github.com/pascualchavez/teleport/internal/config"
	"github.com/pascualchavez/teleport/internal/git"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/pascualchavez/teleport/internal/tui"
	"github.com/spf13/cobra"
)

var beamThenSync bool

var beamCmd = &cobra.Command{
	Use:   "beam [profile]",
	Short: "󰜘 send selected local commits to the remote server",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runBeam,
}

func init() {
	beamCmd.Flags().BoolVarP(&beamThenSync, "then-sync", "s", false, "run sync after beam (working-tree changes over the just-beamed snapshot)")
}

func runBeam(cmd *cobra.Command, args []string) error {
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

	commits, err := git.CommitsAhead()
	if err != nil {
		if errors.Is(err, git.ErrNoUpstream) {
			return fmt.Errorf("beam requires an upstream branch (try `git push -u origin <branch>`)")
		}
		return err
	}
	if len(commits) == 0 {
		fmt.Println("Nothing to beam — no local commits ahead of upstream.")
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

	changes, err := tui.RunBeamFilePicker(allChanges)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		fmt.Println("No files selected.")
		return nil
	}

	hosts, err := sshpkg.ParseSSHConfig()
	if err != nil {
		return fmt.Errorf("parse ssh config: %w", err)
	}

	var targetHost *sshpkg.Host
	for _, h := range hosts {
		if h.Name == profile.Host {
			hCopy := h
			targetHost = &hCopy
			break
		}
	}
	if targetHost == nil {
		targetHost = &sshpkg.Host{
			Name:     profile.Host,
			Hostname: profile.Host,
			Port:     "22",
		}
	}

	log.Info("Connecting", "host", targetHost.Name)
	client, err := sshpkg.Connect(*targetHost)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", targetHost.Name, err)
	}
	defer client.Close()

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
