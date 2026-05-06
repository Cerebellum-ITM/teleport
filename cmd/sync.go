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

const (
	iconSyncOK   = "✓"
	iconSyncFail = "✗"
)

var syncCmd = &cobra.Command{
	Use:   "sync [profile]",
	Short: "Sync git-tracked files to the remote server",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSync,
}

func runSync(cmd *cobra.Command, args []string) error {
	// Determine profile
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

	log.Info("Sync", "profile", profileName, "host", profile.Host, "path", profile.Path)

	// Collect files
	tracked, err := git.TrackedFiles()
	if err != nil {
		return fmt.Errorf("git ls-files: %w", err)
	}
	if len(tracked) == 0 {
		return fmt.Errorf("no tracked files found (is this a git repository?)")
	}

	untracked, err := git.UntrackedFiles()
	if err != nil {
		log.Warn("Could not list untracked files", "err", err)
		untracked = nil
	}

	// File selection TUI
	files, err := tui.RunFilePicker(tracked, untracked)
	if err != nil {
		return err
	}

	log.Info("Files selected", "count", len(files))

	// Parse SSH hosts to find the matching host entry
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
		// Fallback: treat profile.Host as raw hostname
		targetHost = &sshpkg.Host{
			Name:     profile.Host,
			Hostname: profile.Host,
			Port:     "22",
		}
	}

	// Connect
	log.Info("Connecting", "host", targetHost.Name)
	client, err := sshpkg.Connect(*targetHost)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", targetHost.Name, err)
	}
	defer client.Close()

	// Upload files
	fmt.Printf("\nSyncing %d file(s) to %s:%s\n\n", len(files), profile.Host, profile.Path)

	var failed int
	for _, f := range files {
		remotePath := filepath.Join(profile.Path, f)
		if err := client.UploadFile(f, remotePath); err != nil {
			log.Error(iconSyncFail+" "+f, "err", err)
			failed++
		} else {
			fmt.Printf("  %s %s\n", iconSyncOK, f)
		}
	}

	fmt.Println()
	if failed > 0 {
		return fmt.Errorf("%d file(s) failed to upload", failed)
	}

	log.Info("Sync complete", "files", len(files))
	return nil
}
