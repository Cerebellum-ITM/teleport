package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	"github.com/charmbracelet/log"
	"github.com/pascualchavez/teleport/internal/bindetect"
	"github.com/pascualchavez/teleport/internal/config"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/pascualchavez/teleport/internal/tui"
	"github.com/spf13/cobra"
)

var initProfileFlag string

const (
	initTargetSync       = "sync"
	initTargetBinLinux   = "bin-linux"
	initTargetBinMacOS   = "bin-macos"
	initTargetBinWindows = "bin-windows"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: " configure a sync and/or bin profile (SSH host + remote directory)",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringVarP(&initProfileFlag, "profile", "p", "", "sync profile name (default: current dir name)")
}

func runInit(cmd *cobra.Command, args []string) error {
	targets, err := pickInitTargets()
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("nothing to configure — select at least one option")
	}

	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("load global config: %w", err)
	}

	hosts, err := sshpkg.ParseSSHConfig()
	if err != nil {
		return fmt.Errorf("parse ssh config: %w", err)
	}

	doSync := contains(targets, initTargetSync)

	if doSync {
		if err := configureSyncProfile(globalCfg, hosts); err != nil {
			return err
		}
	}

	for _, t := range targets {
		osName, ok := binTargetOS(t)
		if !ok {
			continue
		}
		if err := configureBinProfile(globalCfg, hosts, osName); err != nil {
			return err
		}
	}

	if err := config.SaveGlobal(globalCfg); err != nil {
		return fmt.Errorf("save global config: %w", err)
	}

	if err := configureLocalBinDir(); err != nil {
		return err
	}

	return nil
}

// configureLocalBinDir asks the user for the local bin directory
// (where built binaries live). Autodetects ./bin as the default suggestion.
func configureLocalBinDir() error {
	suggestion := ""
	if info, err := os.Stat("bin"); err == nil && info.IsDir() {
		suggestion = "./bin"
	}

	binDir := suggestion
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Local bin directory").
			Description("Where your built binaries live (leave empty to skip)").
			Placeholder("./bin").
			Value(&binDir),
	))
	if err := form.Run(); err != nil {
		return fmt.Errorf("form: %w", err)
	}
	binDir = strings.TrimSpace(binDir)

	localCfg, err := config.LoadLocal()
	if err != nil {
		return fmt.Errorf("load local config: %w", err)
	}
	localCfg.BinDir = binDir
	if err := config.SaveLocal(localCfg); err != nil {
		return fmt.Errorf("save local config: %w", err)
	}

	if binDir != "" {
		fmt.Printf("\nLocal bin directory set to: %s\n", binDir)
	}
	return nil
}

func pickInitTargets() ([]string, error) {
	var picks []string
	form := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("What do you want to configure?").
			Description("Use space to toggle, enter to confirm").
			Options(
				huh.NewOption("Sync profile (teleport sync / beam / pull / status / clean)", initTargetSync),
				huh.NewOption("Bin profile — Linux  (teleport ship)", initTargetBinLinux),
				huh.NewOption("Bin profile — macOS  (teleport ship)", initTargetBinMacOS),
				huh.NewOption("Bin profile — Windows (teleport ship)", initTargetBinWindows),
			).
			Value(&picks).
			Validate(func(v []string) error {
				if len(v) == 0 {
					return fmt.Errorf("select at least one")
				}
				return nil
			}),
	))
	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("form: %w", err)
	}
	return picks, nil
}

func binTargetOS(target string) (string, bool) {
	switch target {
	case initTargetBinLinux:
		return string(bindetect.Linux), true
	case initTargetBinMacOS:
		return string(bindetect.MacOS), true
	case initTargetBinWindows:
		return string(bindetect.Windows), true
	}
	return "", false
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func configureSyncProfile(globalCfg *config.GlobalConfig, hosts []sshpkg.Host) error {
	profileName := initProfileFlag
	if profileName == "" {
		cwd, _ := os.Getwd()
		profileName = filepath.Base(cwd)
	}

	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Sync profile name").
			Description("Identifier for this sync target").
			Value(&profileName).
			Validate(func(s string) error {
				s = strings.TrimSpace(s)
				if s == "" {
					return fmt.Errorf("profile name cannot be empty")
				}
				return nil
			}),
	))
	if err := form.Run(); err != nil {
		return fmt.Errorf("form: %w", err)
	}
	profileName = strings.TrimSpace(profileName)

	log.Info("Pick SSH host for sync profile")
	host, err := tui.RunHostPicker(hosts)
	if err != nil {
		return err
	}

	client, err := connectToHost(*host)
	if err != nil {
		return err
	}
	defer client.Close()
	log.Info("Connected", "host", host.Name)

	remotePath, err := tui.RunDirPickerWith(client, "/", "  Select sync directory")
	if err != nil {
		return err
	}

	globalCfg.SetProfile(profileName, config.Profile{
		Host: host.Name,
		Path: remotePath,
	})

	localCfg, err := config.LoadLocal()
	if err != nil {
		return fmt.Errorf("load local config: %w", err)
	}
	localCfg.DefaultProfile = profileName
	if err := config.SaveLocal(localCfg); err != nil {
		return fmt.Errorf("save local config: %w", err)
	}

	log.Info("Sync profile saved", "profile", profileName, "host", host.Name, "path", remotePath)
	fmt.Printf("\nSync profile %q configured:\n  host: %s\n  path: %s\n",
		profileName, host.Name, remotePath)
	return nil
}

func configureBinProfile(globalCfg *config.GlobalConfig, hosts []sshpkg.Host, osName string) error {
	log.Info("Pick SSH host for bin profile", "os", osName)
	host, err := tui.RunHostPicker(hosts)
	if err != nil {
		return err
	}

	client, err := connectToHost(*host)
	if err != nil {
		return err
	}
	defer client.Close()
	log.Info("Connected", "host", host.Name)

	startPath := "/usr/local/bin"
	if osName == string(bindetect.Windows) {
		startPath = "/"
	}
	header := fmt.Sprintf("  Select bin/ directory for %s", osName)
	binPath, err := tui.RunDirPickerWith(client, startPath, header)
	if err != nil {
		return err
	}

	globalCfg.SetBinProfile(osName, config.BinProfile{
		Host:    host.Name,
		BinPath: binPath,
	})

	log.Info("Bin profile saved", "os", osName, "host", host.Name, "bin_path", binPath)
	fmt.Printf("\nBin profile %q configured:\n  host:     %s\n  bin_path: %s\n",
		osName, host.Name, binPath)
	return nil
}
