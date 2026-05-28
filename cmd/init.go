package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
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

// multiSelectKeyMap returns a huh KeyMap where Tab is removed from the
// MultiSelect Next binding so Tab never accidentally submits the form.
// Space (and x) remain the toggle keys.
func multiSelectKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.MultiSelect.Next = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "confirm"),
	)
	return km
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

	if contains(targets, initTargetSync) {
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

func pickInitTargets() ([]string, error) {
	var picks []string
	form := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("What do you want to configure?").
			Description("space = toggle  ·  enter = confirm").
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
	)).WithKeyMap(multiSelectKeyMap())
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

	// Optional: fixed remote filename for this OS profile.
	remoteName := ""
	{
		form := huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title(fmt.Sprintf("Remote name for %s (optional)", osName)).
				Description("Filename the binary will have on the remote (e.g. mycli). Leave empty to use the local filename.").
				Value(&remoteName),
		))
		if err := form.Run(); err != nil {
			return fmt.Errorf("form: %w", err)
		}
		remoteName = strings.TrimSpace(remoteName)
	}

	// Optional: local binary file for this OS profile.
	binFile := autodetectBinFile(osName)
	{
		form := huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title(fmt.Sprintf("Local binary for %s (optional)", osName)).
				Description("Path to the local binary (e.g. ./bin/mycli_linux_amd64). Leave empty to pick on each run.").
				Placeholder(binFile).
				Value(&binFile),
		))
		if err := form.Run(); err != nil {
			return fmt.Errorf("form: %w", err)
		}
		binFile = strings.TrimSpace(binFile)
	}

	globalCfg.SetBinProfile(osName, config.BinProfile{
		Host:       host.Name,
		BinPath:    binPath,
		RemoteName: remoteName,
		BinFile:    binFile,
	})

	log.Info("Bin profile saved", "os", osName, "host", host.Name, "bin_path", binPath)
	msg := fmt.Sprintf("\nBin profile %q configured:\n  host:     %s\n  bin_path: %s\n",
		osName, host.Name, binPath)
	if remoteName != "" {
		msg += fmt.Sprintf("  remote_name: %s\n", remoteName)
	}
	if binFile != "" {
		msg += fmt.Sprintf("  bin_file: %s\n", binFile)
	}
	fmt.Print(msg)
	return nil
}

// autodetectBinFile scans ./bin for a file whose name contains the OS name
// and returns the path, or "" if nothing matches.
func autodetectBinFile(osName string) string {
	info, err := os.Stat("bin")
	if err != nil || !info.IsDir() {
		return ""
	}
	entries, err := os.ReadDir("bin")
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.Contains(strings.ToLower(e.Name()), osName) {
			return filepath.Join("./bin", e.Name())
		}
	}
	return ""
}

// configureLocalBinDir asks the user for the local bin directory.
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
