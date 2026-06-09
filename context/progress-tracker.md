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
- **Unit 13 — Beam branch picker**: `teleport beam --branch/-b <branch>` flag to specify source branch; new branch picker TUI when flag omitted and multiple branches exist (current pre-selected); new `git.LocalBranches()` + `git.CommitsAheadOf(branch)` helpers with upstream/remote-only logic; new `internal/tui/branchpicker.go` bubbletea model (`context/specs/13-beam-branch-picker.md`)
- **Unit 14 — Ship**: `teleport ship <bin>` deploys a local executable to a remote `bin/` dir. New `internal/bindetect` package sniffs ELF/Mach-O/PE magic bytes to pick the target OS; `GlobalConfig.BinProfiles` map (keyed by `linux|macos|windows`, validated on load) holds per-OS `{host, bin_path}`; `ssh.Client.RemoteWritable` probes the destination and `ship` falls back to `sudo -n` then masked-password `sudo -S` via new `ssh.Client.RunCommandStdin` when needed; `--os`/`--to`/`--name` flags override detection, path and remote basename; `teleport init` gains a huh multi-select that lets the user configure sync + any subset of bin profiles in one session (reusing the host picker per target and a header-aware `tui.RunDirPickerWith`); `teleport profiles` now prints `Sync profiles` and `Bin profiles` sections (`context/specs/14-ship.md`)

## In Progress

- None.

## Next Up

- (sin unidad planificada)

## Unit 14 Summary

- **Unit 14 — Ship**: new top-level `teleport ship <bin>` subcommand for deploying built CLI binaries to a remote `bin/` directory in three automated steps: SFTP upload → `chmod +x` → `mv` into place (with sudo escalation when the SSH user can't write the destination). Target OS is detected from magic bytes (ELF→linux, Mach-O incl. fat→macos, MZ/PE→windows) by the new `internal/bindetect` package and selects one of up to three bin profiles stored in `~/.config/teleport/config.toml` under `[bin_profiles.<os>]`. Bin profiles live alongside sync profiles in `GlobalConfig.BinProfiles` (`map[string]BinProfile`, validated on load — unknown OS keys raise a decode error). `internal/ssh/client.go` adds `RemoteWritable` (POSIX `[ -w dir ]` probe) and `RunCommandStdin` (used to feed the sudo password via stdin so it never appears in the remote process list). Overwrite is silent (`mv -f`). When sudo prompts and the user cancels, the binary is left under `/tmp/teleport-ship-<pid>-<ts>/<name>` and the message points there. `teleport init` was rewritten around a `huh.MultiSelect` that lets the user pick any combination of `Sync profile` and `Bin profile — {Linux, macOS, Windows}`; the sync flow is preserved when chosen and per-OS bin flows reuse the host picker plus a header-parameterised `tui.RunDirPickerWith` (defaults: `/usr/local/bin` for linux/macos, `/` for windows). `teleport profiles` now prints two labeled sections, omitting either when empty. Help table and examples in `cmd/help.go` updated; no new third-party deps.

## Unit 13 Summary

- **Unit 13 — Beam branch picker**: `teleport beam` now accepts `--branch/-b` flag to specify the source branch for commits. If not provided and multiple branches exist, a new TUI branch picker opens (pre-selecting current branch). If only one branch exists, the picker is skipped. New `git.LocalBranches()` helper returns current branch first, followed by others; new `git.CommitsAheadOf(branch)` uses explicit upstream check or `--not --remotes` fallback. New `internal/tui/branchpicker.go` implements single-select bubbletea model with keys `↑/k`, `↓/j`, `enter`, `ctrl+c`. `cmd/beam.go` adds `resolveBranch` helper and refactored commit sourcing. Message now shows `"Nothing to beam — no local commits on <branch> ahead of remote."` when sync'd. No new dependencies.

## Open Questions

- Should `teleport sync` preserve file permissions on the remote, or always use default SFTP permissions?
- ~~Should the dir browser show hidden directories (dotfiles) as an opt-in?~~ Resolved: show all dirs unconditionally — needed to navigate to paths like `~/.local/bin`.
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
- 2026-05-28 — Unit 14 implemented: `teleport ship`. Pure orchestration in `cmd/ship.go` (Stat → bindetect → resolve `BinProfile[os]` → `connectToHost` → `UploadFile` to `/tmp/teleport-ship-<pid>-<ts>/<name>` → `chmod +x` → `RemoteWritable` probe → `mv -f` plain or via `sudo -n`/`sudo -S` with masked `huh` password). `internal/bindetect/bindetect.go` is a single-purpose magic-byte sniffer (8 bytes read, table match, returns `ErrUnknownFormat` otherwise). `config.LoadGlobal` validates every `bin_profiles.<key>` against `bindetect.Valid`. `internal/tui/dirpicker.go` was refactored to thread a header through `NewDirPickerWith` / `RunDirPickerWith`; the old constructors keep their previous string ("  Remote Directory Browser") for back-compat. `cmd/init.go` was rebuilt around `huh.NewMultiSelect` with at-least-one validator; the sync sub-flow now uses `RunDirPickerWith(client, "/", "  Select sync directory")` and the bin sub-flow uses per-OS start paths plus `"  Select bin/ directory for <os>"`. `cmd/profiles.go` prints separate `Sync profiles` and `Bin profiles` sections. Smoke-tested: `teleport ship ./teleport` without config returns the expected `no bin profile configured for macos …` message; `teleport ship ./go.mod` rejects with `not a recognised executable binary`.
- 2026-06-02 — Fix (Unit 14 follow-up): `teleport ship` dejaba archivos de 0 bytes reportando éxito. Causa: `MaxPacketUnchecked(1<<22)` (4 MiB) excedía `SFTP_MAX_MSG_LENGTH` de OpenSSH (256 KB) → el server descartaba el payload; y `defer dst.Close()` ignoraba el error que el server solo reporta en el close. Fixes en `internal/ssh/client.go`: paquete a `MaxPacket(32768)`; `UploadFile`/`UploadFileProgress`/`UploadBytes` cierran el handle explícitamente y propagan el error de `Close()`; nuevo helper `verifyUpload` compara stat remoto vs tamaño local; `progressReader` expone `Size() int64` para que `sftp.File.ReadFrom` use la ruta concurrente (pipelined) en vez de la secuencial; y `ConnectWithPassword` pasa de `MaxPacketChecked(1<<20)` (rechazado por ser >32 KB, rompía el auth por contraseña) a `MaxPacket(32768)`. Nuevo invariante #6 en `architecture.md`. Verificado manualmente por el usuario (subida correcta y más rápida). No es unidad nueva: no requiere spec. Commit `0fd1e0f`.
- 2026-06-09 — Fix (TUI list overflow): los pickers basados en cursor renderizaban **todos** los ítems de golpe, así que listas largas (p.ej. muchos commits en `beam`) desbordaban la altura de la terminal y rompían la UI visualmente. Nuevo helper compartido `internal/tui/listwindow.go` (`computeWindow` + `scrollUpHint`/`scrollDownHint`) que calcula una ventana deslizante que mantiene el cursor visible y reserva 2 filas para indicadores `↑ N more` / `↓ N more`. Aplicado a `commitpicker`, `beamfilepicker`, `branchpicker`, `localfilepicker`, `dirpicker` y `filepicker` (este último reparte el alto entre las secciones tracked/untracked vía `sectionRows`, priorizando la lista interactiva). Cada modelo gana un campo `height` (default 24) actualizado vía `tea.WindowSizeMsg`. Test unitario `listwindow_test.go` cubre los casos límite de `computeWindow`. No es unidad nueva: fix visual a features existentes, sin spec. Sin dependencias nuevas.
- 2026-05-28 — Unit 13 enhancement: branch picker gains always-on filter input (`textinput.Model` focused on open, type to filter, `esc` clears, `↑↓`/`ctrl+p`/`ctrl+n` navigate filtered list, `enter` selects highlighted match) and Nerd Font icons (`󰘬 ` md-source_branch for branches, `󰋜 ` md-home for `main`/`master`). Spec doc added at `context/specs/13-beam-branch-picker.md`.
