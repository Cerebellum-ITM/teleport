# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.1.1] - 2026-05-06

### Fixed

- Fixed SSH authentication exhaustion by limiting the keys offered from the SSH agent to those matching the `IdentityFile` defined in `ssh_config`.
- Improved agent signer selection using fingerprint comparison to filter out non-matching keys.
- Enhanced `IdentityFile` support by allowing `.pub` file paths.

## [Unreleased]

### Added
- `teleport version` subcommand — prints version, short commit hash, and build timestamp
- `internal/version` package with `Version`, `Commit`, and `Date` variables injected at build time via `-ldflags`
- `Makefile` now derives version from `git describe --tags --always --dirty` and injects it on every `make build` and `make install`

### Fixed
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
