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

## In Progress

- None.

## Next Up

- _TBD: confirm with user_ — possible next features: diff-based sync (only changed files), watch mode, SSH password auth, `teleport remove` to delete a profile.

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
