package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	"github.com/charmbracelet/log"
	"github.com/pascualchavez/teleport/internal/config"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/pascualchavez/teleport/internal/tui"
	"github.com/spf13/cobra"
)

var initProfileFlag string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Configure a sync profile (SSH host + remote directory)",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringVarP(&initProfileFlag, "profile", "p", "", "profile name (default: current dir name)")
}

func runInit(cmd *cobra.Command, args []string) error {
	// Step 1: profile name via huh form
	profileName := initProfileFlag
	if profileName == "" {
		cwd, _ := os.Getwd()
		profileName = filepath.Base(cwd)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Profile name").
				Description("Identifier for this sync target").
				Value(&profileName).
				Validate(func(s string) error {
					s = strings.TrimSpace(s)
					if s == "" {
						return fmt.Errorf("profile name cannot be empty")
					}
					return nil
				}),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("form: %w", err)
	}
	profileName = strings.TrimSpace(profileName)

	// Step 2: host picker
	log.Info("Parsing SSH config...")
	hosts, err := sshpkg.ParseSSHConfig()
	if err != nil {
		return fmt.Errorf("parse ssh config: %w", err)
	}

	host, err := tui.RunHostPicker(hosts)
	if err != nil {
		return err
	}

	log.Info("Connecting", "host", host.Name)
	client, err := sshpkg.Connect(*host)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", host.Name, err)
	}
	defer client.Close()
	log.Info("Connected", "host", host.Name)

	// Step 3: dir picker
	startPath := "/"
	remotePath, err := tui.RunDirPicker(client, startPath)
	if err != nil {
		return err
	}

	// Save config
	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("load global config: %w", err)
	}

	globalCfg.SetProfile(profileName, config.Profile{
		Host: host.Name,
		Path: remotePath,
	})

	if err := config.SaveGlobal(globalCfg); err != nil {
		return fmt.Errorf("save global config: %w", err)
	}

	localCfg := &config.LocalConfig{DefaultProfile: profileName}
	if err := config.SaveLocal(localCfg); err != nil {
		return fmt.Errorf("save local config: %w", err)
	}

	log.Info("Profile saved", "profile", profileName, "host", host.Name, "path", remotePath)
	fmt.Printf("\nProfile %q configured:\n  host: %s\n  path: %s\n",
		profileName, host.Name, remotePath)

	return nil
}
