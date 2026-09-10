package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/spf13/cobra"
)

// shellCandidates are probed in order on the remote; the first one installed
// wins.
var shellCandidates = []string{"zsh", "bash", "sh"}

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

	argv := []string{"ssh", "-t", profile.Host, remoteShellCommand(profile.Path)}
	return execSSH(sshBin, argv)
}

// remoteShellCommand builds the snippet ssh runs on the remote: cd into dir
// when the profile has one, then exec the first shell that exists. Each
// candidate is probed with `command -v` because a failed `exec` kills the
// login shell instead of falling through to the next one.
func remoteShellCommand(dir string) string {
	var b strings.Builder
	if dir != "" {
		b.WriteString("cd " + sshpkg.ShellQuote(dir) + " || exit 1; ")
	}
	for _, sh := range shellCandidates {
		b.WriteString("command -v " + sh + " >/dev/null 2>&1 && exec " + sh + "; ")
	}
	b.WriteString("echo 'teleport: no shell found on the remote (tried " +
		strings.Join(shellCandidates, ", ") + ")' >&2; exit 127")
	return b.String()
}
