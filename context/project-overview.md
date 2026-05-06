# teleport

## Overview

`teleport` is a developer CLI tool that synchronizes files tracked by git (plus
optional untracked extras) to a remote server via SSH/SFTP before committing.
It targets developers who iterate on remote machines — staging servers, VPS,
homelab — and want to test changes live without polluting git history with
post-deploy `FIX` commits. The tool reads `~/.ssh/config` so no extra
credentials configuration is required.

## Goals

1. A developer can configure a sync profile (host + remote path) in under 60 seconds using the interactive TUI.
2. Running `teleport sync` uploads all git-tracked files to the configured remote in one command.
3. Multiple profiles can coexist globally; any project selects its default via `.teleport.toml`.

## Core User Flow

1. Developer runs `teleport init` inside a project directory.
2. Enters a profile name (defaults to the current directory name).
3. Picks an SSH host from a filterable list sourced from `~/.ssh/config`.
4. Browses the remote filesystem via SFTP and selects the target directory.
5. Profile is saved to `~/.config/teleport/config.toml`; `.teleport.toml` is created in CWD.
6. Developer makes local code changes.
7. Runs `teleport sync` — sees git-tracked files pre-selected; optionally toggles untracked extras.
8. Confirms → files are uploaded with per-file ✓/✗ feedback.
9. Iterates until satisfied, then commits cleanly.

## Features

### Profile Management

- `teleport init` — interactive 3-step TUI: profile name → host picker → remote dir browser
- `teleport profiles` — list all configured profiles, marking the current project default with `*`
- Global config at `~/.config/teleport/config.toml`; local override at `.teleport.toml`

### File Sync

- `teleport sync [profile]` — upload git-tracked files to the remote profile path
- Multi-select TUI to toggle untracked files in/out of the sync set
- Per-file success/failure log with Nerd Font icons
- Remote directories created automatically (MkdirAll) if they don't exist

### SSH / SFTP

- Reads `~/.ssh/config` for host resolution (Hostname, User, Port)
- Auth via ssh-agent (`SSH_AUTH_SOCK`) or default key files (`id_ed25519`, `id_rsa`, `id_ecdsa`)
- Falls back to raw hostname when host not in `~/.ssh/config`

## Scope

### In Scope

- Sync git-tracked files + optional untracked extras
- Interactive profile setup (host picker, remote dir browser)
- Multiple named profiles, one default per project
- SSH public-key auth only (agent + key files)
- File upload; remote directory creation

### Out of Scope

- Two-way sync or remote → local pull
- Password-based SSH authentication
- Non-git projects (no `git ls-files` equivalent planned)
- Watch mode / automatic sync on file change
- Diff-based sync (only changed files) — currently uploads all selected files every run
- Web UI or daemon mode

## Success Criteria

1. `teleport init` completes in under 60 s on a machine with `~/.ssh/config` entries and a reachable host.
2. `teleport sync` uploads all files returned by `git ls-files` with zero failures on a clean network connection.
3. `teleport profiles` lists all profiles defined in `~/.config/teleport/config.toml` with the local default marked.
4. Running `teleport sync staging` (explicit profile) overrides the `.teleport.toml` default without error.
5. `go build -o teleport .` produces a working binary with no compilation errors.
