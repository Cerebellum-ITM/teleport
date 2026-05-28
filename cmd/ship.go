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
	"github.com/pascualchavez/teleport/internal/tui"
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

var (
	shipOKStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	shipDimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	shipBoldStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
)

// suppress unused import warning when log is only used in TouchLastSync path
var _ = log.Info

func init() {
	shipCmd.Flags().StringVar(&shipOS, "os", "", "override target OS (linux|macos|windows)")
	shipCmd.Flags().StringVar(&shipTo, "to", "", "override remote bin directory for this run")
	shipCmd.Flags().StringVar(&shipName, "name", "", "rename binary on the remote (overrides profile remote_name)")
	rootCmd.AddCommand(shipCmd)
}

func runShip(_ *cobra.Command, args []string) error {
	localPath, profile, err := resolveShipContext(args)
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

	// Determine target OS (flag > auto-detect).
	targetOS := bindetect.OS(shipOS)
	if targetOS == "" {
		targetOS, err = bindetect.Detect(localPath)
		if err != nil {
			return fmt.Errorf("detect binary type: %w", err)
		}
	} else if !bindetect.Valid(string(targetOS)) {
		return fmt.Errorf("invalid --os %q (expected linux|macos|windows)", shipOS)
	}

	// If resolveShipContext gave us a profile already (from BinFile match),
	// verify its OS matches; otherwise look up by targetOS.
	if profile == nil {
		globalCfg, err := config.LoadGlobal()
		if err != nil {
			return fmt.Errorf("load global config: %w", err)
		}
		p, ok := globalCfg.BinProfiles[string(targetOS)]
		if !ok {
			return fmt.Errorf("no bin profile configured for %s — run \"teleport init\" and add one", targetOS)
		}
		profile = &p
	}

	// Resolve destination dir and remote filename.
	binDir := profile.BinPath
	if shipTo != "" {
		binDir = shipTo
	}
	remoteName := resolveRemoteName(localPath, profile)

	host, err := resolveSSHHost(profile.Host)
	if err != nil {
		return err
	}
	client, err := connectToHost(host)
	if err != nil {
		return err
	}
	defer client.Close()

	start := time.Now()
	tmpDir := fmt.Sprintf("/tmp/teleport-ship-%d-%d", os.Getpid(), time.Now().UnixNano())
	localBasename := filepath.Base(localPath)
	tmpUploadPath := tmpDir + "/" + localBasename // upload keeps original filename
	tmpFinalPath := tmpDir + "/" + remoteName      // after rename (same if no rename)
	finalPath := binDir + "/" + remoteName
	needsRename := remoteName != localBasename

	moveCmd := fmt.Sprintf("mv -f %s %s",
		sshpkg.ShellQuote(tmpFinalPath), sshpkg.ShellQuote(finalPath))
	usedSudo := false

	header := fmt.Sprintf("Shipping %s → %s:%s", localBasename, profile.Host, finalPath)

	stepNames := []string{fmt.Sprintf("uploading   %s", localBasename)}
	if needsRename {
		stepNames = append(stepNames, fmt.Sprintf("renaming    %s → %s", localBasename, remoteName))
	}
	stepNames = append(stepNames,
		"chmod +x",
		fmt.Sprintf("moving   → %s:%s", profile.Host, finalPath),
	)

	steps := []tui.ShipStepFunc{
		func(setExtra func(string)) error {
			return client.UploadFileProgress(localPath, tmpUploadPath, func(written, total int64) {
				pct := 0
				if total > 0 {
					pct = int(written * 100 / total)
				}
				setExtra(fmt.Sprintf("  %s / %s  %d%%",
					tui.HumanBytes(written), tui.HumanBytes(total), pct))
			})
		},
	}
	if needsRename {
		renameCmd := fmt.Sprintf("mv %s %s",
			sshpkg.ShellQuote(tmpUploadPath), sshpkg.ShellQuote(tmpFinalPath))
		steps = append(steps, func(_ func(string)) error {
			_, e := client.RunCommand(renameCmd)
			return e
		})
	}
	steps = append(steps,
		func(_ func(string)) error {
			_, e := client.RunCommand("chmod +x " + sshpkg.ShellQuote(tmpFinalPath))
			return e
		},
		func(_ func(string)) error {
			writable, e := client.RemoteWritable(binDir)
			if e != nil {
				return fmt.Errorf("probe %s: %w", binDir, e)
			}
			if writable {
				_, e = client.RunCommand(moveCmd)
				return e
			}
			usedSudo = true
			return moveWithSudo(client, host, moveCmd, profile.Host, tmpFinalPath)
		},
	)

	errs, tuiErr := tui.RunShipProgress(header, stepNames, steps)
	if tuiErr != nil {
		return tuiErr
	}
	_, _ = client.RunCommand("rmdir " + sshpkg.ShellQuote(tmpDir))

	for i, e := range errs {
		if e != nil {
			return fmt.Errorf("step %d (%s): %w", i+1, stepNames[i], e)
		}
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	sudoTag := ""
	if usedSudo {
		sudoTag = shipDimStyle.Render(" (sudo)")
	}
	fmt.Printf("  %s  %s → %s%s\n",
		shipOKStyle.Render("shipped"),
		shipBoldStyle.Render(remoteName),
		shipBoldStyle.Render(profile.Host+":"+finalPath),
		sudoTag,
	)
	fmt.Printf("  %s  %s\n",
		shipDimStyle.Render(string(targetOS)),
		shipDimStyle.Render(elapsed.String()),
	)

	if err := config.TouchLastSync(); err != nil {
		log.Warn("could not update last sync timestamp", "err", err)
	}
	return nil
}

// resolveShipContext returns the local binary path and, when a profile
// BinFile is configured and no explicit arg was passed, also pre-resolves
// the matching BinProfile (to avoid a second lookup by OS later).
func resolveShipContext(args []string) (string, *config.BinProfile, error) {
	// Explicit arg always wins; profile lookup happens by OS later.
	if len(args) > 0 {
		return args[0], nil, nil
	}

	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return "", nil, fmt.Errorf("load global config: %w", err)
	}

	// If any bin profile has a BinFile configured, try to auto-resolve.
	// If --os is set, narrow to that profile; otherwise try all profiles.
	for osKey, p := range globalCfg.BinProfiles {
		if shipOS != "" && osKey != shipOS {
			continue
		}
		if p.BinFile != "" {
			pCopy := p
			return p.BinFile, &pCopy, nil
		}
	}

	// Fall back to bin-dir / picker.
	localCfg, err := config.LoadLocal()
	if err != nil {
		return "", nil, fmt.Errorf("load local config: %w", err)
	}
	if localCfg.BinDir == "" {
		return "", nil, fmt.Errorf("no binary specified and no bin-dir or bin_file configured\nhint: run `teleport config set bin-dir ./bin` or pass the binary path directly")
	}

	entries, err := os.ReadDir(localCfg.BinDir)
	if err != nil {
		return "", nil, fmt.Errorf("read bin-dir %s: %w", localCfg.BinDir, err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	if len(files) == 0 {
		return "", nil, fmt.Errorf("no files found in bin-dir %s", localCfg.BinDir)
	}
	if len(files) == 1 {
		return filepath.Join(localCfg.BinDir, files[0]), nil, nil
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
		return "", nil, fmt.Errorf("binary picker: %w", err)
	}
	return chosen, nil, nil
}

// resolveRemoteName picks the destination filename in priority order:
// --name flag > profile.RemoteName > local basename.
func resolveRemoteName(localPath string, profile *config.BinProfile) string {
	if shipName != "" {
		return shipName
	}
	if profile.RemoteName != "" {
		return profile.RemoteName
	}
	return filepath.Base(localPath)
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

	sudoSCmd := "sudo -S -p '' -- sh -c " + sshpkg.ShellQuote(moveCmd)
	if _, err := client.RunCommandStdin(sudoSCmd, pw+"\n"); err != nil {
		return fmt.Errorf("sudo mv failed: %w", err)
	}
	return nil
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
