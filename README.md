# teleport

CLI tool that syncs git-tracked files to a remote server via SSH/SFTP before
you commit — for developers who iterate on remote machines (staging servers,
VPS, homelab) and want to test live without polluting git history.

```
teleport init      # one-time profile setup (host + remote path)
teleport sync      # upload changed files → iterate → commit clean
```

## How it works

1. `teleport init` — interactive TUI: pick an SSH host from `~/.ssh/config`,
   browse the remote filesystem, save a named profile.
2. Make local code changes.
3. `teleport sync` — uploads files changed since last commit (`git diff HEAD`)
   with a live progress bar. Add `-u` to include untracked files too.
4. Iterate until satisfied, then commit cleanly.

## Commands

| Command | Description |
|---|---|
| `teleport init` | Interactive profile setup (host picker → remote dir browser) |
| `teleport sync` | Upload changed tracked files to the default profile |
| `teleport sync <profile>` | Override the default profile |
| `teleport sync -u` | Also include untracked files |
| `teleport profiles` | List all configured profiles (`*` marks the local default) |
| `teleport version` | Print version, commit hash, and build date |

Root shorthands: `-s` (sync), `-i` (init), `-p` (profiles), `-u` (untracked).
`teleport -su` is equivalent to `teleport sync -u`.

## Installation

**Requirements:** Go 1.25+, a terminal with a [Nerd Font](https://www.nerdfonts.com/) installed.

```sh
git clone https://github.com/pascualchavez/teleport
cd teleport
make build       # builds ./bin/teleport and copies to ~/.local/bin
```

For cross-platform release binaries:

```sh
make build_release   # darwin_arm64, darwin_amd64, linux_amd64, linux_arm64 → ./bin/
```

## SSH authentication

Teleport reads `~/.ssh/config` for host resolution (Hostname, User, Port).
Authentication uses ssh-agent (`SSH_AUTH_SOCK`) when available, otherwise falls
back to default key files (`id_ed25519`, `id_rsa`, `id_ecdsa`). When
`IdentityFile` is set in `~/.ssh/config`, only that key is offered to avoid
exhausting `MaxAuthTries`.

Password-based auth is not supported.

## Configuration

- Global profiles: `~/.config/teleport/config.toml`
- Local project default: `~/.config/teleport/projects/<sha256-of-cwd>.toml`

No config files are placed inside your project directories.

## Tech stack

Built with the [Charm](https://charm.sh/) v2 TUI stack:
[Bubbletea](https://github.com/charmbracelet/bubbletea) ·
[Bubbles](https://github.com/charmbracelet/bubbles) ·
[Lipgloss](https://github.com/charmbracelet/lipgloss) ·
[Huh](https://github.com/charmbracelet/huh) · SFTP via
[pkg/sftp](https://github.com/pkg/sftp).
