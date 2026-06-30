# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Fixed
- The `teleport beam` file picker's commit count no longer silently undercounts. When several selected commits rework the same files, `git.FilesInCommits` dedupes each path to the newest commit that touched it, so older commits that own no surviving file used to vanish from the count line — selecting 5 commits could read "3 commits" with no explanation, even though all 5 are still beamed and credited as sent. The picker now tracks the selected-commit total separately from the contributing commits and surfaces the gap explicitly: it reads `5 commits · 3 with files` when they differ, and the plain `5 commits` when every selected commit owns a file. The filter-by-commit (`←/→`) still steps only through commits that have visible files.

## [0.7.0] - 2026-06-29

### Added
- The `teleport beam` file picker can now preview a file before sending it: with the cursor on a file, `v` opens the full file (the blob at that file's commit) and `d` opens the diff that commit introduced — both in a `bat`-style pager with syntax highlighting and a line-number gutter. Inside the viewer `tab` switches file ⇄ diff, scroll is `j/k`/`↑↓`/`ctrl+d`/`ctrl+u`/`g`/`G`, and `esc`/`q` returns to the picker with the cursor and selection intact. Viewing is orthogonal to selection — it never changes what gets beamed. Deleted files show their pre-delete contents in file mode and the deletion hunk in diff mode; binary blobs show a placeholder. Highlighting is done by a new `internal/highlight` package wrapping chroma (pure Go, no CGO), with the Catppuccin-Mocha style.
- The diff view is rendered **delta-style**: the code on each line is syntax-highlighted, with a tinted two-column (old/new) line-number gutter marking added (green), removed (red), and context (dim) lines. Each hunk opens with a full-width bar separating the `@@` line range (dim) from the enclosing function/class context (accent), with a blank line between hunks. The redundant unified-diff file header (`diff --git`/`index`/`---`/`+++`) is hidden — the viewer header already shows the path and commit.

## [0.6.0] - 2026-06-29

### Added
- The `teleport beam` commit picker now lets you manually mark/unmark commits as "already beamed" without re-sending: `m` toggles the sent badge of the commit under the cursor (live), and `M` toggles all commits at once (symmetric — marks all, or unmarks all if every commit is already marked). This corrects the sent-tracking for commits that are genuinely on the remote but show as pending because identity is the exact SHA (new branches with no history, rebased/amended/cherry-picked commits, or the first beam from another machine). Marks accumulate during the picker session and are persisted to the per-profile beamed store only on `enter` (the write happens as soon as the picker confirms, so marks survive even if the file picker or upload is later cancelled); `ctrl+c` discards them. The delta (added + removed SHAs) is applied in a single atomic write via the new `LocalConfig.ApplyBeamedDelta`, and removals are scoped to the commits actually shown so stale SHAs in history are never disturbed. `beam --auto` skips the picker, so manual marking is inert there by design.

### Documentation
- Added animated demo GIFs for every command and embedded them in the README: a hero GIF (`profiles` → `sync`), inline GIFs in the `beam`/`ship`/`shell` sections, and a collapsible "Every command in action" gallery (`init`, `sync`, `status`, `clean`, `pull`, `profiles`, `config`, `version`/`--help`). The GIFs are recorded with [VHS](https://github.com/charmbracelet/vhs) and are fully simulated: a `teleport()` shell function under `demo/sim/` renders each command's real styled output — colors and Nerd Font glyphs extracted byte-for-byte from the Go source via `gen-glyphs.sh` — using invented data only (no network, no real config, no private paths). The demo tooling lives in `demo/` (`sim/`, `tapes/`, `gifs/`) with `demo/README.md` documenting how to regenerate the GIFs with `vhs`.

## [0.5.0] - 2026-06-17

### Added
- `teleport shell [profile]` — opens an interactive shell on the remote, already `cd`'d into the profile's remote path. It derives the host and path from the existing sync profile (no extra configuration) and runs `ssh -t <host> "cd '<path>' && exec zsh"`. The command replaces its own process with the system `ssh` binary (`syscall.Exec`), so it behaves exactly like a hand-typed `ssh` — native TTY, colors, agent, `~/.ssh/config` — and leaves no teleport process running while the session is open. The existing `sync`/`beam`/`ship`/`clean`/`pull` commands are unaffected; the new code is isolated to `cmd/shell.go` (with a `syscall.Exec` Unix path and an `exec.Command` Windows fallback).

## [0.4.0] - 2026-06-11

### Added
- `teleport beam` gained an `--auto`/`-a` flag that skips the commit picker entirely: it auto-selects exactly the commits not yet beamed to the active profile (the same set the picker pre-selects) and goes straight to the file review. When every local commit ahead of the remote was already sent, it prints `Nothing to beam — all local commits on <branch> already sent.` and exits.
- New root-level shortcut `teleport -b`/`--beam` runs the beam flow without typing the `beam` subcommand, matching the existing `-s`/`-i`/`-p` shortcuts. It combines with the auto shortcut as `teleport -ba` (a root-level `-a`/`--auto` flag bound to the same behavior). The full `teleport beam` subcommand remains the only way to pass beam's own sub-flags (`--branch`, `--clean`, `--then-sync`, `--yes`).

### Changed
- The beam send/progress view is now grouped by commit instead of a flat file list. Each commit gets a colored header (`󰆧 [shortSHA] ─ subject`) using the same per-commit accent as the file picker, with its files listed underneath marked `✓` uploaded / `✗` failed / `·` pending. This replaces the previous layout that repeated the colored cube and `[shortSHA]` on every file line, so the short SHA and subject now appear once per commit. The visible window follows the most recently completed file so progress stays on screen. Plain `teleport sync` keeps its flat streaming log. Internally, `BeamMarker`/`RunSyncProgressMarked` were replaced by `BeamGroup`/`RunBeamSendProgress` in `internal/tui/syncprogress.go`.

## [0.3.0] - 2026-06-09

### Changed
- Selecting/deselecting items is now `tab` everywhere, consistently. The sync file picker (which only accepted `space`) and the `teleport init` profile multi-select now toggle with `tab`; `space` still works as an alias. The commit picker and beam file picker already used `tab`. File/directory browsers are unaffected — there `tab` descends into a directory (there is nothing to toggle).

### Added
- `teleport beam` now remembers which commits were already beamed to each profile and shows it in the commit picker. Already-sent commits get a green sent badge (`󰗠`) and a dimmed subject; the picker opens with only the not-yet-sent commits pre-selected, and a new `u` key re-selects exactly the unsent set (`a` still toggles all). "Sent" is tracked per destination profile (a commit beamed to `production` is still unsent for `staging`) and persisted in the per-project local config (`[beamed_commits.<profile>]`). A commit is recorded as sent only when every path it touched was uploaded/deleted without error — attribution is by path, not by the winning blob, so a commit whose file was superseded by a newer commit is still credited (and partial failures reappear next run). SHAs that are no longer ahead of the remote (pushed, merged, or rebased) are pruned automatically.

### Improved
- `teleport beam` file picker ("Files from selected commits") now groups files by the commit they come from and color-codes each group, so it reads at a glance which file belongs to which commit. Every file line is prefixed with a colored cube (`󰆧`) and its path is tinted in the commit's color; files are ordered by commit (then alphabetically within each commit), and deletions keep their red `(delete)` marker. Each commit gets a distinct accent from a documented 16-color palette (`beamCommitPalette`), cycling when there are more commits than colors.
- The beam send/progress view now tags each file with its commit's colored cube (`󰆧`) and short SHA (e.g. `[a1b2c3]`), matching the file picker's per-commit coloring, so you can tell at a glance which commit each uploaded file came from. Colors are computed from the same change set the picker used, so they stay consistent between the two views.
- The beam file picker can now filter by commit with ←/→. The default view ("all commits") shows every file with a count + filter hint; pressing →/← cycles to a single commit, collapsing the header to one line (colored cube + short SHA + subject + `i/N` position) and showing only that commit's files. This keeps the picker usable with many commits — previously a long commit legend pushed the file list off-screen (e.g. 33 commits left a single visible file row). File selections persist across filters, and `a` toggles all files in the currently active filter.

### Fixed
- Interactive list pickers no longer break the terminal layout when the list is longer than the visible screen. Previously every picker rendered all items at once, so long lists (e.g. many commits in `teleport beam`) overflowed the terminal height and scrolled the header/footer off-screen. All cursor-driven pickers (commit picker, beam file picker, branch picker, local file picker, remote dir picker) and the sync file picker now render a sliding window sized to the terminal height, keeping the cursor in view and showing `↑ N more` / `↓ N more` indicators for the hidden items. The sync file picker splits the available height between its tracked and untracked sections, favoring the interactive list. Implemented via a shared `computeWindow` helper in `internal/tui/listwindow.go`; each picker model now tracks terminal height via `tea.WindowSizeMsg`.

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
