# Progress Tracker

Update this file after every meaningful implementation change.

## Current Phase

- Brownfield adoption — initial implementation complete, context files being established.

## Current Goal

- Maintain and extend the existing CLI (add features, fix issues as they surface).

## Completed

- `internal/config` — GlobalConfig + LocalConfig TOML read/write
- `internal/ssh` — `~/.ssh/config` parser, SSH+SFTP connection, `UploadFile`, `ListDirs`
- `internal/git` — `TrackedFiles()`, `UntrackedFiles()` via `git ls-files`
- `internal/tui/hostpicker` — filterable bubbles/list of SSH hosts
- `internal/tui/dirpicker` — remote directory browser via SFTP
- `internal/tui/filepicker` — multi-select file picker (tracked pre-selected, untracked toggleable)
- `cmd/init` — 3-step flow: huh form → host picker → dir browser → save config
- `cmd/sync` — file picker TUI → SFTP upload with per-file ✓/✗ log
- `cmd/profiles` — list profiles with local default marked
- `cmd/root` + `main.go` — cobra wiring, `-v` flag, binary entry point
- **Unit 03 — sync flag defaults**: `LocalConfig.SyncUntracked`, `teleport config get|set|unset` subcommand, and post-sync warning when untracked files are skipped (`context/specs/03-sync-flag-defaults.md`)
- **Unit 04 — beam (commit-driven sync)**: `teleport beam` subcommand, commit picker + beam file picker TUIs, `git.CommitsAhead/FilesInCommits/FileAtCommit`, `ssh.UploadBytes/Remove` (`context/specs/04-beam-commits.md`)
- **Unit 05 — status**: `teleport status [profile]` with `--pending`/`-p`, `ssh.Client.RemoteSHA256`, full and pending-mode classification (`==`, `!=`, `??`, `--`), exit code 1 on drift (`context/specs/05-status.md`)
- **Unit 06 — file icons**: extended `fileTypeIcon` in `internal/tui/syncprogress.go` with Nerd Font glyphs for ~60 extensions plus basename matches for `Dockerfile`/`Makefile`/`.gitignore` (`context/specs/06-file-icons.md`)
- **Unit 07 — clean**: `teleport clean [profile]` discards dirty changes on the remote git work tree (`git checkout -- . && git clean -fd`), with `--ignored`/`-x` to also wipe gitignored files; `beam --clean`/`-c` chains it before the beam (`beam -cs` = clean → beam → sync); new `ssh.Client.RunCommand` + `ssh.ShellQuote` helpers; new `tui.RunCleanConfirm` (`context/specs/07-clean.md`)
- **Unit 08 — config polish**: extracted reusable `helpDoc`/`renderHelp` in `cmd/help.go` and applied styled help (badge title, sections, Nerd Font icons, Examples, new `Config keys` table) to `teleport config` and `get|set|unset`; `teleport -h` output stays byte-identical; `teleport config get` (no args) now shows the resolved profile (host:path), explicit `<unset>` / `(from global)` / `(not found in global config)` states, and a `last sync` block with absolute timestamp + Spanish humanized "hace …"; `LocalConfig.LastSync time.Time` + `config.TouchLastSync()` persisted on successful `sync`, `beam`, and `clean`; `config get <key>` unchanged for scripts (`context/specs/08-config-polish.md`)
- **Unit 09 — SSH password auth**: `ErrNoAuthMethods` sentinel in `internal/ssh/client.go`; `ConnectWithPassword(host, pw)`; `connectToHost` in `cmd/clean.go` catches the sentinel and calls `promptPassword` (masked `huh` input); `connectToProfile` delegates to `connectToHost`; `init`, `sync`, `status` migrated to use the shared helpers — all commands now fall back to password prompt when no key/agent is available (`context/specs/09-ssh-password-auth.md`)
- **Unit 10 — Remove profile**: `teleport profiles remove <name>` (alias `rm`) elimina un profile del global config; `GlobalConfig.RemoveProfile` helper en `internal/config/config.go`; warning si el profile eliminado era el `default-profile` del local config actual, sin modificar el local config (`context/specs/10-remove-profile.md`)
- **Unit 12 — Pull**: `teleport pull [profile]` downloads files changed on the remote to local working tree; pre-checks ensure local working tree is clean and both remotes are at the same commit; uses `git status --porcelain=v1 -z` to detect modified/added/deleted/untracked files; deletes via `os.Remove`, downloads via SFTP with per-file ✓/-/✗ output; `git.HasUncommittedChanges()` + `git.LocalHEAD()` helpers; `ssh.Client.DownloadFile()` method (`context/specs/12-pull.md`)

## In Progress

- None.

## Next Up

- (sin unidad planificada)

## Open Questions

- Should `teleport sync` preserve file permissions on the remote, or always use default SFTP permissions?
- Should the dir browser show hidden directories (dotfiles) as an opt-in?
- Is `InsecureIgnoreHostKey` fallback acceptable for first-connection UX, or should we prompt to add the key?

## Architecture Decisions

- **bubbletea v2 over v1**: v2 changes `Init()` to return `tea.Cmd` (not `(Model, Cmd)`) and `View()` to return `tea.View` (not string). All models must use `tea.NewView(s)`.
- **No CGO**: binary must cross-compile; all dependencies are pure Go.
- **BurntSushi/toml for config**: chosen over `encoding/json` for human-readable, comment-friendly config files that users may edit manually.
- **`InsecureIgnoreHostKey` fallback**: used when `~/.ssh/known_hosts` is missing or unreadable, to avoid blocking the init flow on first use. Acceptable for a local developer tool; not acceptable for production services.

## Session Notes

- Initial implementation produced 2026-05-06.
- All 14 files committed in a single `[ADD] teleport` root commit (hash `1fbf8f2`).
- Context files scaffolded via `/spec-driven-dev init --from-code` in the same session.
- 2026-05-12 — Unit 03 implemented: warning emitted **after** the sync TUI exits (rendering full-height inline view would otherwise scroll the warning off-screen if printed before).
- 2026-05-15 — Unit 04 implemented: `teleport beam`. Per-file content fetched with `git show <sha>:<path>` from the most-recent selected commit that touched the file; deletes via `SFTP.Remove` (idempotent on missing). Reused existing `SyncProgress` TUI for the upload phase.
- 2026-05-15 — Added `--then-sync`/`-s` flag to `beam`: chains a working-tree sync on the same SSH connection after the beam phase succeeds. Order is fixed (beam → sync) so disk content always wins over the beamed blob.
- 2026-05-15 — Unit 05 implemented: `teleport status`. SHA256 streamed over SFTP per file; no remote state persisted. `--pending` reuses `git.CommitsAhead`/`FilesInCommits` and flags `'D'` paths still present on the remote with `--`.
- 2026-05-15 — Unit 06 implemented: expanded `fileTypeIcon` map. Codepoints chosen via the `nerd-fonts` skill (seti-*, dev-*, md-*, cod-*, custom-elixir). Basename switch handles `Dockerfile`, `Makefile`, `.gitignore`/`.gitattributes`/`.gitmodules` before the extension lookup. Fallback unchanged (cod-file).
- 2026-05-15 — Unit 07 implemented: `teleport clean`. Pure SSH (no SFTP) — runs `git rev-parse --is-inside-work-tree` as a safe guard, parses `git status --porcelain=v1 -z` (with `--ignored` when `-x` is set), shows a bubbletea confirm grouped by revert/remove/restore/ignored, then runs `git checkout -- . && git clean -fd [-x]`. `beam -c` reuses the helper and the open connection; refactored `beam` profile resolution and connect into shared `resolveProfile` / `connectToProfile` helpers in `cmd/clean.go`.
- 2026-05-25 — Unit 08 implemented: `config` polish. Refactor in `cmd/help.go` extracts `helpDoc`/`renderHelp`; verified byte-identical `teleport -h` against pre-refactor binary via temp clone + diff. `config`, `config get`, `set`, `unset` each register `SetHelpFunc` with their own `helpDoc` (own title, tagline, examples) and share a `Config keys` table. `config get` (no args) now prints a styled overview: explicit `<unset>` / `(from global)` for `default-profile`, resolved `host:path`, and `last sync` (absolute `YYYY-MM-DD HH:MM:SS -ZZ` + Spanish "hace N minutos/horas/días/MMM D"). `config get <key>` left untouched for `$()` use. `LocalConfig.LastSync time.Time` (omitempty) persisted by `config.TouchLastSync()` — called at the end of `sync`, `beam`, and `clean` (incl. the already-clean path); failures are warn-logged, not fatal.
- 2026-05-27 — Unit 12 implemented: `teleport pull [profile]`. Adds `git.HasUncommittedChanges()` and `git.LocalHEAD()` helpers; adds `ssh.Client.DownloadFile()` for SFTP downloads with local parent dir creation and partial-write cleanup; new `cmd/pull.go` subcommand orchestrates pre-checks (local working tree clean, remote at same commit), fetches remote dirty files via `git status --porcelain=v1 -z`, and downloads/deletes with styled per-file output. Added pull to help output in `cmd/help.go`. No new dependencies.
