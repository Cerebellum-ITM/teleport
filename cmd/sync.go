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
		fmt.Println("Nothing to sync — no changes since last commit.")
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

	header := fmt.Sprintf("Syncing %d file(s) to %s:%s", len(changed), profile.Host, profile.Path)
	failed, err := tui.RunSyncProgress(header, changed, func(localPath string) error {
		return client.UploadFile(localPath, filepath.Join(profile.Path, localPath))
	})
	if err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("%d file(s) failed to upload", failed)
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
