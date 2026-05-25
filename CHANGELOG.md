# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.1.1] - 2026-05-08

### Added

- Added a comprehensive `README.md` that outlines the project description, usage flow, command reference table, installation steps, SSH authentication setup, configuration profiles, and the tech stack.
- Documented installation commands and required build targets in the `README.md`.
- Listed supported tech-stack components and their versions in the `README.md`.
- Provided guidance on SSH key handling and profile configuration in the `README.md`.

## [Unreleased]

### Added
- `teleport profiles remove <name>` (alias `rm`) removes a profile from the global config. Exits with a clear error and hint when the profile does not exist. Emits a warning (but does not modify the local config) when the removed profile was the `default-profile` of the current directory. Implemented via `GlobalConfig.RemoveProfile` in `internal/config` and a new `profilesRemoveCmd` in `cmd/profiles.go`.
- SSH password auth fallback: when no SSH agent is running and no key files are found, all commands that connect to a remote server now prompt for a password instead of aborting. Implemented via `ErrNoAuthMethods` sentinel in `internal/ssh`, `ConnectWithPassword`, and `connectToHost`/`promptPassword` helpers in `cmd/`.
- `teleport config` and subcommands (`get`, `set`, `unset`) now render styled help with badge title, section headers, Nerd Font icons, and a `Config keys` table — replaces cobra's plain-text default.
- `teleport config get` (no args) shows the active profile with resolved `host` and `path`, plus `last sync` in absolute and human-relative format (Spanish, e.g. "hace 4 horas"). A single-key invocation (`config get default-profile`) still prints only the raw value for scripting compatibility.
- `LastSync time.Time` field added to `LocalConfig` (persisted as `last_sync` in TOML); `TouchLastSync()` helper updates it atomically.
- `teleport sync`, `teleport beam`, and `teleport clean` call `TouchLastSync()` on successful completion so `config get` always reflects the last remote interaction.
- `humanizeSince` helper in the `cmd` package renders Spanish relative timestamps with cutoffs: seconds / minutes / hours / days / absolute date.
- `teleport clean [profile]` subcommand — discards dirty changes on the remote git working tree by running `git checkout -- .` and `git clean -fd` over SSH. Validates that `profile.Path` is a git work tree (`git rev-parse --is-inside-work-tree`); aborts with a hint if not. Renders a confirmation TUI grouping changes by *revert* (modified), *remove* (untracked), *restore* (deleted), and *remove ignored* (only with `-x`). Flags: `--yes`/`-y` to skip the prompt, `--ignored`/`-x` to also delete gitignored files (passes `-x` to `git clean`).
- `teleport beam --clean`/`-c` — runs the same clean phase before the beam (using the same SSH session); `beam -cs` chains clean → beam → sync. `beam -y` skips the clean prompt. Works without commits ahead of upstream (acts as a pure clean).
- `internal/ssh.Client.RunCommand` — execute a remote command over a fresh SSH session, returning stdout and wrapping stderr into the error.
- `internal/ssh.ShellQuote` — POSIX single-quote helper for safe path embedding in remote commands.
- `internal/tui.RunCleanConfirm` — bubbletea confirmation TUI for the clean plan, reusing the existing `fileTypeIcon` to render per-extension glyphs.
- Sync progress icon map expanded: dedicated Nerd Font glyphs for `xml`, `svg`, `toml`, `ini`, `env`, `conf`, `cfg`, `lock`, shells (`sh`/`bash`/`zsh`/`fish`/`ps1`/`bat`), frontend (`jsx`/`tsx`/`vue`/`svelte`/`scss`/`sass`/`less`), languages (`c`/`h`/`cpp`/`cc`/`hpp`/`java`/`kt`/`swift`/`rb`/`php`/`lua`/`dart`/`ex`/`exs`), data (`sql`/`csv`/`tsv`/`db`/`sqlite`), text (`txt`/`log`/`rst`), images (`png`/`jpg`/`jpeg`/`gif`/`webp`/`ico`/`bmp`), archives (`zip`/`tar`/`gz`/`tgz`/`7z`/`rar`/`pdf`/`exe`/`bin`), plus full-basename matches for `Dockerfile`, `Makefile`, `.gitignore`, `.gitattributes`, `.gitmodules`.
- `teleport status [profile]` subcommand — compares local files against the remote via SHA256 over SFTP and reports drift. Default mode checks all `git ls-files`; `--pending`/`-p` checks only files in commits ahead of upstream + working-tree changes (+ untracked when persisted), and flags remote files that should have been deleted by a beamed commit.
- `teleport beam` subcommand — pick local commits ahead of upstream and send their file contents to the remote (cherry-pick style). Multi-select TUI for commits + file picker pre-selected; honors per-commit file content via `git show`, removes deleted files on remote.
- `teleport beam --then-sync` / `-s` — after beaming the selected commits, run a working-tree sync over the same SSH connection so dirty (uncommitted) changes overwrite the beamed snapshot.
- `teleport config get|set|unset` subcommand to persist per-working-directory defaults (currently `sync-untracked` and `default-profile`) in the existing local TOML
- `sync_untracked` field in `LocalConfig`; when `true`, `teleport sync` includes untracked files without requiring `-u`
- `teleport sync` warns when untracked files exist and were not included, suggesting `-u` or persisting the default via `teleport config set sync-untracked true`
- `README.md` — project overview, commands, installation, SSH auth, and configuration reference
- `bin/` added to `.gitignore` — binaries are never committed
- `Makefile` `build` target now outputs to `./bin/teleport` and hot-copies it to `~/.local/bin` for local dev iteration
- `Makefile` `build_release` target produces four binaries (`darwin_arm64`, `darwin_amd64`, `linux_amd64`, `linux_arm64`) in `./bin/` with `-trimpath`
- `teleport sync` progress bar: bubbletea TUI with file log scrolling above and ASCII `[======>   ]` bar pinned to the last three lines of the terminal, showing `N/Total  %  MM:SS`
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
