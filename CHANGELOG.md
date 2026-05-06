# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- `teleport sync` now uploads only files changed since the last commit (`git diff --name-only HEAD`) instead of all tracked files — removes the file-picker TUI from the normal sync flow
- `-u`/`--untracked` flag on `teleport sync` to also include untracked files alongside changed tracked files
- Root-level shorthand flags: `-s` (sync), `-i` (init), `-p` (profiles), `-u` (include untracked) — e.g. `teleport -su` is equivalent to `teleport sync -u`
- Custom lipgloss help with Nerd Font icons, harmonious color palette, and an Examples section — replaces cobra's default help for the root command
- `teleport version` subcommand — prints version, short commit hash, and build timestamp
- `internal/version` package with `Version`, `Commit`, and `Date` variables injected at build time via `-ldflags`
- `Makefile` now derives version from `git describe --tags --always --dirty` and injects it on every `make build` and `make install`
- Dir browser now has a live text filter — type to narrow visible directories, `esc` clears it, `backspace` on empty filter navigates up
- Local config stored at `~/.config/teleport/projects/<sha256-of-cwd>.toml` — no more `.teleport.toml` in project directories

### Fixed
- Dir browser: `tab`/`→` descend, `shift+tab`/`←` go up, `enter` confirms selection (removed `s` key)
- `teleport init` no longer prints stale "Add .teleport.toml to .gitignore" hint
- Host picker: filter no longer starts blank — all hosts are visible as soon as the picker opens
- SSH authentication: agent (1Password, ssh-agent) used exclusively when `SSH_AUTH_SOCK` is set — avoids `Too many authentication failures` on servers with low `MaxAuthTries` limits
- SSH authentication: when `IdentityFile` is set in `~/.ssh/config`, only the matching agent signer is offered (fingerprint comparison) — prevents exhausting `MaxAuthTries` when the agent holds many keys
- `IdentityFile` now accepts both the private key path and the `.pub` path directly (1Password exports only the public key)
- Host picker: filter input is active from launch; `q` key only quits when not in filter mode

## [0.1.0] - 2026-05-06

### Added
- `teleport init` — interactive 3-step flow (profile name → SSH host picker → remote dir browser) that saves a `.teleport.toml` local config
- `teleport sync` — file picker TUI (tracked files pre-selected, untracked toggleable) followed by SFTP upload with per-file ✓/✗ status log
- `teleport profiles` — list all configured profiles; marks the local default with `*`
- SSH/SFTP upload via golang.org/x/crypto/ssh and github.com/pkg/sftp
- Bubbletea v2 TUI components (host picker, dir browser, file picker)
- `Makefile` with `build`, `install` (`~/.local/bin`), `uninstall`, and `clean` targets
