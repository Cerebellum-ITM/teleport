package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"charm.land/huh/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/log"
	"github.com/pascualchavez/teleport/internal/bindetect"
	"github.com/pascualchavez/teleport/internal/config"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/spf13/cobra"
)

var (
	shipOS   string
	shipTo   string
	shipName string
)

var shipCmd = &cobra.Command{
	Use:   "ship [bin]",
	Short: " deploy a local binary to its OS-matching bin profile",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runShip,
}

var shipOKStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))

func init() {
	shipCmd.Flags().StringVar(&shipOS, "os", "", "override target OS (linux|macos|windows)")
	shipCmd.Flags().StringVar(&shipTo, "to", "", "override remote bin directory for this run")
	shipCmd.Flags().StringVar(&shipName, "name", "", "rename binary on the remote")
	rootCmd.AddCommand(shipCmd)
}

func runShip(_ *cobra.Command, args []string) error {
	localPath, err := resolveShipBin(args)
	if err != nil {
		return err
	}

	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", localPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", localPath)
	}

	targetOS := bindetect.OS(shipOS)
	if targetOS == "" {
		targetOS, err = bindetect.Detect(localPath)
		if err != nil {
			return fmt.Errorf("detect binary type: %w", err)
		}
	} else if !bindetect.Valid(string(targetOS)) {
		return fmt.Errorf("invalid --os %q (expected linux|macos|windows)", shipOS)
	}

	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("load global config: %w", err)
	}
	profile, ok := globalCfg.BinProfiles[string(targetOS)]
	if !ok {
		return fmt.Errorf("no bin profile configured for %s — run \"teleport init\" and add one", targetOS)
	}

	binDir := profile.BinPath
	if shipTo != "" {
		binDir = shipTo
	}
	remoteName := shipName
	if remoteName == "" {
		remoteName = filepath.Base(localPath)
	}

	host, err := resolveSSHHost(profile.Host)
	if err != nil {
		return err
	}
	client, err := connectToHost(host)
	if err != nil {
		return err
	}
	defer client.Close()

	tmpDir := fmt.Sprintf("/tmp/teleport-ship-%d-%d", os.Getpid(), time.Now().UnixNano())
	tmpPath := tmpDir + "/" + remoteName
	log.Info("Uploading", "to", profile.Host+":"+tmpPath)
	if err := client.UploadFile(localPath, tmpPath); err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	if _, err := client.RunCommand("chmod +x " + sshpkg.ShellQuote(tmpPath)); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	finalPath := binDir + "/" + remoteName
	writable, err := client.RemoteWritable(binDir)
	if err != nil {
		return fmt.Errorf("probe %s: %w", binDir, err)
	}

	moveCmd := fmt.Sprintf("mv -f %s %s",
		sshpkg.ShellQuote(tmpPath), sshpkg.ShellQuote(finalPath))

	usedSudo := false
	if writable {
		if _, err := client.RunCommand(moveCmd); err != nil {
			return fmt.Errorf("mv: %w", err)
		}
	} else {
		usedSudo = true
		if err := moveWithSudo(client, host, moveCmd, profile.Host, tmpPath); err != nil {
			return err
		}
	}

	_, _ = client.RunCommand("rmdir " + sshpkg.ShellQuote(tmpDir))

	suffix := string(targetOS)
	if usedSudo {
		suffix += ", sudo"
	}
	fmt.Printf("%s shipped %s → %s:%s (%s)\n",
		shipOKStyle.Render(""), remoteName, profile.Host, finalPath, suffix)
	return nil
}

// resolveSSHHost looks up name in ~/.ssh/config; falls back to a raw
// hostname Host struct when not found.
func resolveSSHHost(name string) (sshpkg.Host, error) {
	hosts, err := sshpkg.ParseSSHConfig()
	if err != nil {
		return sshpkg.Host{}, fmt.Errorf("parse ssh config: %w", err)
	}
	for _, h := range hosts {
		if h.Name == name {
			return h, nil
		}
	}
	return sshpkg.Host{Name: name, Hostname: name, Port: "22"}, nil
}

// moveWithSudo tries `sudo -n` first; on failure prompts the user with a
// masked password input and pipes it to `sudo -S`. On cancellation, the
// binary is left under tmpPath on the remote and the user is told where.
func moveWithSudo(client *sshpkg.Client, host sshpkg.Host, moveCmd, hostName, tmpPath string) error {
	sudoCmd := "sudo -n -- sh -c " + sshpkg.ShellQuote(moveCmd)
	if _, err := client.RunCommand(sudoCmd); err == nil {
		return nil
	}

	pw, err := promptSudoPassword(host)
	if err != nil {
		return fmt.Errorf("aborted — binary left at %s:%s", hostName, tmpPath)
	}

	// -S reads password from stdin; -p '' suppresses the prompt label.
	sudoSCmd := "sudo -S -p '' -- sh -c " + sshpkg.ShellQuote(moveCmd)
	if _, err := client.RunCommandStdin(sudoSCmd, pw+"\n"); err != nil {
		return fmt.Errorf("sudo mv failed: %w", err)
	}
	return nil
}

// resolveShipBin returns the local binary path to ship.
// If an explicit arg was given, use it directly.
// Otherwise, read bin_dir from local config and pick from the files there.
func resolveShipBin(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}

	localCfg, err := config.LoadLocal()
	if err != nil {
		return "", fmt.Errorf("load local config: %w", err)
	}
	if localCfg.BinDir == "" {
		return "", fmt.Errorf("no binary specified and no bin-dir configured\nhint: run `teleport config set bin-dir ./bin` or pass the binary path directly")
	}

	entries, err := os.ReadDir(localCfg.BinDir)
	if err != nil {
		return "", fmt.Errorf("read bin-dir %s: %w", localCfg.BinDir, err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	if len(files) == 0 {
		return "", fmt.Errorf("no files found in bin-dir %s", localCfg.BinDir)
	}
	if len(files) == 1 {
		return filepath.Join(localCfg.BinDir, files[0]), nil
	}

	var chosen string
	opts := make([]huh.Option[string], len(files))
	for i, f := range files {
		opts[i] = huh.NewOption(f, filepath.Join(localCfg.BinDir, f))
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title(fmt.Sprintf("Select binary from %s", localCfg.BinDir)).
			Options(opts...).
			Value(&chosen),
	))
	if err := form.Run(); err != nil {
		return "", fmt.Errorf("binary picker: %w", err)
	}
	return chosen, nil
}

func promptSudoPassword(host sshpkg.Host) (string, error) {
	var pw string
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title(fmt.Sprintf("sudo password for %s@%s", host.User, host.Hostname)).
			EchoMode(huh.EchoModePassword).
			Value(&pw),
	))
	if err := form.Run(); err != nil {
		return "", errors.New("password prompt cancelled")
	}
	return pw, nil
}
