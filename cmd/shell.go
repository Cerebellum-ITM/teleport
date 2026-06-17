package cmd

import (
	"fmt"
	"os/exec"

	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/spf13/cobra"
)

// shellRemoteShell is the interactive shell launched on the remote.
const shellRemoteShell = "zsh"

var shellCmd = &cobra.Command{
	Use:   "shell [profile]",
	Short: " open an interactive shell on the remote at the profile's path",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runShell,
}

func init() {
	rootCmd.AddCommand(shellCmd)
}

// runShell resolves the profile, then replaces the teleport process with the
// system ssh binary so the session behaves exactly like a hand-typed ssh and
// no teleport process lingers while it is open.
func runShell(_ *cobra.Command, args []string) error {
	profile, _, err := resolveProfile(args)
	if err != nil {
		return err
	}

	sshBin, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh binary not found in PATH: %w", err)
	}

	remoteCmd := "exec " + shellRemoteShell
	if profile.Path != "" {
		remoteCmd = "cd " + sshpkg.ShellQuote(profile.Path) + " && exec " + shellRemoteShell
	}

	argv := []string{"ssh", "-t", profile.Host, remoteCmd}
	return execSSH(sshBin, argv)
}
