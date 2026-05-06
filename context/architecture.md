# Architecture Context

## Stack

| Layer        | Technology                            | Role                                              |
| ------------ | ------------------------------------- | ------------------------------------------------- |
| Language     | Go 1.25                               | Single binary, cross-platform CLI                 |
| CLI framework | cobra v1.10                          | Command routing, flag parsing, help generation    |
| TUI framework | charm.land/bubbletea v2.0.6          | Interactive terminal UI (host picker, dir browser, file picker) |
| TUI forms    | charm.land/huh v2.0.0                 | Profile name input form                           |
| TUI components | charm.land/bubbles v2.1.0           | Filterable list component for host picker         |
| TUI styling  | charm.land/lipgloss v2.0.3            | Color tokens and text styling in terminal         |
| Logging      | charmbracelet/log v1.0.0              | Structured terminal log output                    |
| SSH          | golang.org/x/crypto/ssh               | SSH connection, auth (agent + key files)          |
| SFTP         | github.com/pkg/sftp v1.13.10          | Remote file listing and upload                    |
| Config       | github.com/BurntSushi/toml v1.6.0     | Read/write TOML config files                      |

## System Boundaries

- `cmd/` — cobra command definitions; orchestrates packages from `internal/`; owns the user-facing flow
- `internal/config/` — reads and writes global (`~/.config/teleport/config.toml`) and local (`.teleport.toml`) config; no I/O except config files
- `internal/ssh/` — parses `~/.ssh/config`, establishes SSH+SFTP connections, exposes `UploadFile` and `ListDirs`; no config or TUI logic
- `internal/git/` — runs `git ls-files` and `git ls-files --others`; returns plain string slices; no SSH or config logic
- `internal/tui/` — bubbletea models for host picker, dir browser, file picker; pure TUI; no business logic

## Storage Model

- **Global config** (`~/.config/teleport/config.toml`): named profiles with SSH host and remote path. Shared across all projects on the machine.
- **Local config** (`.teleport.toml` in project CWD): one field — `default_profile`. Per-project, not committed to git.
- **No database, no remote state**: all state is in flat TOML files on the local filesystem.

## Auth and Access Model

- Authentication is entirely delegated to the OS SSH stack. `teleport` does not manage credentials.
- Auth precedence: (1) `SSH_AUTH_SOCK` agent, (2) `~/.ssh/id_ed25519`, (3) `~/.ssh/id_rsa`, (4) `~/.ssh/id_ecdsa`.
- Host key verification uses `~/.ssh/known_hosts` when present; falls back to `InsecureIgnoreHostKey` otherwise.
- No user accounts, no tokens, no teleport-specific auth layer.

## Invariants

1. `internal/` packages must never import `cmd/` — dependency flow is strictly `cmd → internal`.
2. SSH and SFTP connections are always created in `cmd/` layer (or delegated to `internal/ssh`), never inside TUI models.
3. `internal/tui` models must not perform I/O directly — they receive data via messages and emit selections; callers do the I/O.
4. Config writes happen only after all interactive steps complete successfully — a cancelled TUI flow must not write partial config.
5. `go build` must produce a zero-dependency static binary (no CGO).
