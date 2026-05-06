# Code Standards

## General

- Keep packages small and single-purpose; each `internal/` package owns one concern.
- Fix root causes — do not add fallback logic that hides real errors.
- Do not mix TUI, SSH, git, and config concerns in the same function.
- No comments unless the WHY is non-obvious (hidden constraint, subtle invariant, workaround). Never describe what the code does.

## Go

- Use `fmt.Errorf("context: %w", err)` for all error wrapping — never discard the original error.
- Prefer named return values only when the function is short and the names add clarity; avoid them in complex functions.
- No `init()` functions outside cobra `cmd/` subcommands.
- No package-level mutable globals except cobra command vars and flag vars in `cmd/`.
- Validate external input at system boundaries (`cmd/` before passing to `internal/`); trust `internal/` functions' own preconditions.
- No `interface{}` / `any` unless required by a third-party API.
- CGO must remain disabled; the binary must cross-compile cleanly.

## cobra (cmd/)

- Each command lives in its own file: `cmd/init.go`, `cmd/sync.go`, `cmd/profiles.go`.
- Command logic goes in `runXxx(cmd, args) error` functions — keep `cobra.Command` declarations as thin wiring.
- Use `cobra.MaximumNArgs` / `cobra.ExactArgs` to reject bad arg counts at the cobra level.
- Flags registered in `init()` of each command file; global flags registered in `cmd/root.go`.

## bubbletea v2 (internal/tui/)

- Model interface: `Init() tea.Cmd`, `Update(tea.Msg) (tea.Model, tea.Cmd)`, `View() tea.View`.
- `View()` always returns `tea.NewView(string)` — never return a raw string.
- TUI models must not perform SSH calls or file I/O inside `Update`. I/O happens via `tea.Cmd` functions that return messages.
- Expose a single `RunXxx()` function per TUI component that creates and runs the program, then extracts the result.

## SSH / SFTP (internal/ssh/)

- `Connect()` always returns a `*Client` that owns both the SSH and SFTP connections; callers must call `client.Close()` via `defer`.
- `UploadFile` creates remote parent directories automatically before writing.
- Auth errors surface verbatim to the user — do not swallow them with generic messages.

## Error Handling

- Command-level errors returned from `RunE` are printed by cobra automatically — do not double-print.
- For partial failures (e.g. one file fails to upload), collect the count and return a final error after all files are attempted.
- Never `os.Exit` inside `internal/` packages — return errors up to `cmd/`.

## File Organization

- `cmd/` — one file per command + `root.go`; no business logic
- `internal/config/` — `config.go` only; types + load/save functions
- `internal/ssh/` — `client.go` only; SSH config parser + connection + upload
- `internal/git/` — `git.go` only; `TrackedFiles()` and `UntrackedFiles()`
- `internal/tui/` — one file per TUI component: `hostpicker.go`, `dirpicker.go`, `filepicker.go`
- `context/` — methodology context files; never imported by Go code
