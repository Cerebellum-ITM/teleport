# Unit 16: Shell — drop into an interactive shell on the remote

## Goal

Add `teleport shell [profile]` so the user can jump straight into an
interactive shell on the remote host, already `cd`'d into the profile's remote
path. No new configuration: the command derives the host and path from the
existing sync profile (the same one `sync`/`beam`/`clean` use).

The command must **replace its own process** with the system `ssh` binary so
that (a) no teleport process lingers while the session is open, and (b) the
session has the exact behaviour and performance of running `ssh` by hand —
native TTY, colors, agent, `~/.ssh/config`, flow control.

## Design

### Subcommand

```
teleport shell [profile]
```

- `[profile]` — optional sync profile name. When omitted, the per-directory
  `default_profile` is used (same resolution as `sync`/`clean`).

The effective command executed is:

```
ssh -t <profile.Host> "cd '<profile.Path>' && exec zsh"
```

- `-t` forces pseudo-TTY allocation (required for an interactive shell).
- `cd '<path>' && exec zsh` lands the session in the remote working directory
  and replaces the login shell with `zsh`, leaving a clean remote process tree.
- `<profile.Host>` is the `~/.ssh/config` alias stored in the profile. It is
  passed straight to the system `ssh`, which resolves it natively — teleport's
  internal SSH-config parser is **not** used for this command.

### Fixed remote shell

The remote shell is fixed to `zsh` (confirmed decision), held in a single
package-level constant `shellRemoteShell = "zsh"` so it is trivial to change
later. If the path is empty for some reason, the remote command degrades to
`exec zsh` (no `cd`).

### Process-replacement model

`teleport shell` does **not** use the pure-Go SSH stack in `internal/ssh`
(that stays the transport for `sync`/`beam`/`ship`/`clean`/`pull` and is left
untouched). Instead it `execve`s the system `ssh`:

- On Unix (`!windows`): `syscall.Exec(sshBin, argv, os.Environ())`. On success
  it never returns — the teleport process image becomes `ssh`. No child, no
  waiting parent, no lingering teleport PID.
- On Windows: `syscall.Exec` is unavailable, so fall back to
  `exec.Command` with inherited stdio and `os.Exit(exitCode)` when it finishes.
  This branch exists only so the package compiles cross-platform; the user
  works on darwin.

### Out of scope

- Configurable shell (zsh is fixed for v1).
- Storing a custom shell command/template per profile (zero-config by
  decision — host+path are derived from the existing profile).
- A separate host/path for the shell distinct from the sync profile.
- Running arbitrary remote commands (`teleport shell -- <cmd>`); this unit is
  interactive-shell only.
- Bin profiles (`teleport ship`) — unrelated.

## Implementation

### `cmd/shell.go` — new file

The whole command. No business logic beyond resolving the profile and building
`argv`:

```go
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

func runShell(_ *cobra.Command, args []string) error {
	profile, _, err := resolveProfile(args) // reuses cmd/clean.go
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
```

### `cmd/shell_unix.go` — new file (`//go:build !windows`)

```go
//go:build !windows

package cmd

import (
	"os"
	"syscall"
)

// execSSH replaces the current process with ssh. On success it never returns,
// so no teleport process lingers while the session is open.
func execSSH(bin string, argv []string) error {
	return syscall.Exec(bin, argv, os.Environ())
}
```

### `cmd/shell_windows.go` — new file (`//go:build windows`)

```go
//go:build windows

package cmd

import (
	"os"
	"os/exec"
)

// execSSH runs ssh as a child with inherited stdio and exits with its code.
// syscall.Exec is unavailable on Windows; this keeps the package compiling.
func execSSH(bin string, argv []string) error {
	c := exec.Command(bin, argv[1:]...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	os.Exit(0)
	return nil
}
```

### `cmd/root.go`

No change needed beyond the `rootCmd.AddCommand(shellCmd)` registered in
`cmd/shell.go`'s `init()` (matches the `ship` pattern, which self-registers).
Do **not** add a root shorthand flag for shell (avoid clashing with `-h`).

### `cmd/help.go`

Add a `shell [profile]` row to the command table with a one-line description,
following the existing lipgloss styling of the other rows.

### Helpers reused (nothing reimplemented)

- `resolveProfile(args) (config.Profile, string, error)` — `cmd/clean.go`.
- `sshpkg.ShellQuote(path)` — `internal/ssh`, already used in `cmd/clean.go`.

## Dependencies

No new third-party packages (uses stdlib `os/exec` and `syscall`).

## Verify when done

- [ ] `go build -o teleport . && go vet ./...` clean.
- [ ] `GOOS=windows go build ./...` compiles (build-tag split is correct).
- [ ] With a default profile configured, `teleport shell` opens an interactive
  `zsh` session sitting in `profile.Path` (verify `pwd` and `echo $0`).
- [ ] `teleport shell <other-profile>` uses that profile's host and path.
- [ ] While the remote shell is open, `pgrep -fl teleport` on the local machine
  shows **no** teleport process (it was replaced by `ssh`); exiting the shell
  returns to the local prompt with no orphaned processes.
- [ ] Responsiveness/latency is indistinguishable from running
  `ssh -t <host> "cd <path> && exec zsh"` by hand.
- [ ] Non-existent profile → clear error from `resolveProfile`.
- [ ] No `ssh` in `PATH` → `ssh binary not found in PATH` error.
- [ ] `teleport shell --help` lists the command and `[profile]` arg.
- [ ] `sync`/`beam`/`ship`/`clean`/`pull` behaviour is unchanged (shell code is
  isolated to the new files).
