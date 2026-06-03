# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.2.0] - 2026-06-02

### Added
- `teleport ship [bin]` — deploys a locally built CLI binary to a remote `bin/` directory in three automated steps: SFTP upload to a `/tmp` staging path → `chmod +x` → `mv` into the target dir (with automatic `sudo` escalation when the SSH user cannot write the destination). Target OS is auto-detected from magic bytes (ELF→linux, Mach-O incl. fat binary→macos, MZ/PE→windows); `--os` overrides detection. `--to` overrides the configured `bin_path` for the current run only. `--name` renames the binary on the remote. When `sudo` is needed, `sudo -n` is tried first; if a password is required a masked `huh` prompt appears, and on cancellation the binary is left under the staging path with a message pointing there. New `internal/bindetect` package handles magic-byte sniffing.
- `bin-dir` local config key (`teleport config set bin-dir ./bin`) — stores the project-local directory where built binaries live. `teleport ship` without an argument reads this key: auto-selects the only file if there is one, or opens a `huh.Select` picker when multiple files are present. Configured interactively in `teleport init`.
- `teleport init` now opens a multi-select at the start so the user can configure any combination of: sync profile, bin profile for Linux, bin profile for macOS, bin profile for Windows. Sync and bin flows are independent and can both be configured in a single session. A final step asks for the local `bin-dir` (autodetects `./bin` if it exists). Bin profiles are stored in `[bin_profiles.<os>]` in the global config; `teleport config set bin-dir` persists the source dir in the local project config.
- `teleport profiles` now prints two labeled sections — `Sync profiles` and `Bin profiles` — omitting either when empty.
- Remote directory browser now shows hidden directories (e.g. `.local`, `.config`) alongside visible ones, enabling navigation to paths like `~/.local/bin`.

### Improved
- `teleport init` remote directory picker accepts a custom header per invocation (`RunDirPickerWith`), so sync and bin flows show contextual titles instead of a generic "Remote Directory Browser".
- `teleport init` bin profile setup now asks for an optional `remote_name` (fixed filename on the remote) and `bin_file` (local binary path, auto-detected from `./bin/<os>` patterns). Tab no longer accidentally submits the multi-select — only Space/x toggle and Enter confirms.
- `teleport ship` resolves the local binary from `profile.bin_file` automatically when configured, skipping the picker on every run. Remote filename priority: `--name` flag > `profile.remote_name` > local basename. Ship prints all steps upfront (ANSI cursor movement updates each line in place — yellow spinner while active, green ✓ on success); the upload line shows a live `[===>   ]` progress bar sized to the terminal width (`TermWidth()` via `TIOCGWINSZ`) plus `X MB / Y MB  N%`. Nerd Font icons per step: `󱓞` header, `󰕒` upload, `󰑕` rename, `󰯄` chmod, `󰔰` move, `󰗠` shipped. When the remote name differs from the local filename, an explicit `renaming  original → remote` step appears between upload and chmod. Upload speed improved: SFTP uses `UseConcurrentWrites` + `sftp.File.ReadFrom` on the fast concurrent (pipelined) path instead of 32 KB sequential `io.Copy` writes.
- `teleport init` bin profile setup now uses a local filesystem TUI picker (navigable with ↑↓, Tab/→ to descend, ← to go up, Enter to select) for choosing the local binary file instead of a text input. The `bin-dir` question is skipped entirely when all configured bin profiles already have a `bin_file` set.

### Fixed
- `teleport ship` no longer leaves a 0-byte file on the remote while reporting success. The SFTP write packet was capped at 4 MiB (`MaxPacketUnchecked(1<<22)`), which exceeds OpenSSH's `SFTP_MAX_MSG_LENGTH` (256 KB) and made the server discard the payload; it is now `MaxPacket(32768)`. Upload functions (`UploadFile`, `UploadFileProgress`, `UploadBytes`) now close the remote handle explicitly and propagate its error (servers often only report write failures on close), and verify the remote size matches the local size before reporting success.
- SFTP uploads now actually use the concurrent (pipelined) path: `progressReader` exposes `Size() int64` so `sftp.File.ReadFrom` no longer falls back to the slow sequential per-ACK write loop.
- Password authentication is functional again: `ConnectWithPassword` used `MaxPacketChecked(1<<20)`, which `MaxPacketChecked` rejects (>32 KB) and caused `sftp.NewClient` to fail; switched to `MaxPacket(32768)`.

## [0.1.4] - 2026-05-28

### Added
- `teleport beam --branch/-b <branch>` flag to specify which local branch to source commits from instead of always using the current branch's upstream. If omitted and multiple local branches exist, a new TUI branch picker opens (with the current branch pre-selected); passing the flag skips the TUI. Single-branch repos skip the picker automatically. Implemented via `git.LocalBranches()` helper (returns current branch first), `git.CommitsAheadOf(branch)` with upstream check or `--not --remotes` fallback, and `internal/tui.RunBranchPicker()` bubbletea single-select model.

### Improved
- Branch picker now opens with an always-active filter input — typing filters the list immediately, `enter` confirms the highlighted match, `esc` clears the filter, and `↑↓`/`ctrl+p`/`ctrl+n` navigate. Branches are prefixed with a Nerd Font glyph (`󰘬 ` md-source_branch); `main`/`master` get a distinct icon (`󰋜 ` md-home).

## [0.1.3] - 2026-05-27

### Added
- `teleport pull [profile]` — downloads files that were modified directly on the remote to the local working tree. Pre-checks: local working tree must be clean and both sides must be at the same commit (aborts with a hint if not). Uses `git status --porcelain=v1 -z` on the remote to identify changed/added/untracked/deleted files; downloads via SFTP, removes locally-deleted files, reports per-file `✓`/`-`/`✗` status and a summary line. Exit code 1 when any file fails. Implemented via `Client.DownloadFile` in `internal/ssh`, `git.HasUncommittedChanges` and `git.LocalHEAD` in `internal/git`, and `cmd/pull.go`.
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

## [0.1.2] - 2026-05-12

### Added
- `teleport config get|set|unset` subcommand to persist per-working-directory defaults (currently `sync-untracked` and `default-profile`) in the existing local TOML
- `sync_untracked` field in `LocalConfig`; when `true`, `teleport sync` includes untracked files without requiring `-u`
- `teleport sync` warns when untracked files exist and were not included, suggesting `-u` or persisting the default via `teleport config set sync-untracked true`

## [0.1.1] - 2026-05-08

### Added
- Added a comprehensive `README.md` that outlines the project description, usage flow, command reference table, installation steps, SSH authentication setup, configuration profiles, and the tech stack.
- Documented installation commands and required build targets in the `README.md`.
- Listed supported tech-stack components and their versions in the `README.md`.
- Provided guidance on SSH key handling and profile configuration in the `README.md`.

## [0.1.0] - 2026-05-06

### Added
- `teleport init` — interactive 3-step flow (profile name → SSH host picker → remote dir browser) that saves a `.teleport.toml` local config
- `teleport sync` — file picker TUI (tracked files pre-selected, untracked toggleable) followed by SFTP upload with per-file ✓/✗ status log
- `teleport profiles` — list all configured profiles; marks the local default with `*`
- SSH/SFTP upload via golang.org/x/crypto/ssh and github.com/pkg/sftp
- Bubbletea v2 TUI components (host picker, dir browser, file picker)
- `Makefile` with `build`, `install` (`~/.local/bin`), `uninstall`, and `clean` targets
