# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.1.1] - 2026-05-06

### Added

- Added a `teleport version` subcommand to output package version information.

## [Unreleased]

### Added
- `teleport version` subcommand — prints version, short commit hash, and build timestamp
- `internal/version` package with `Version`, `Commit`, and `Date` variables injected at build time via `-ldflags`
- `Makefile` now derives version from `git describe --tags --always --dirty` and injects it on every `make build` and `make install`

## [0.1.0] - 2026-05-06

### Added
- `teleport init` — interactive 3-step flow (profile name → SSH host picker → remote dir browser) that saves a `.teleport.toml` local config
- `teleport sync` — file picker TUI (tracked files pre-selected, untracked toggleable) followed by SFTP upload with per-file ✓/✗ status log
- `teleport profiles` — list all configured profiles; marks the local default with `*`
- SSH agent support (1Password and other agents); falls back to key files only when no agent is present
- Host picker with filter input active from launch; `Enter` selects the highlighted match
- `Makefile` with `build`, `install` (`~/.local/bin`), `uninstall`, and `clean` targets
