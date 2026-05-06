# Unit 02: Sync progress bar

## Goal

Replace the plain `fmt.Printf` upload loop in `teleport sync` with a
bubbletea TUI that shows a log of completed files in the upper area and
a fixed ASCII progress bar in the last three lines of the terminal.

## Design

No altscreen (bubbletea v2 doesn't expose `WithAltScreen`). The view always
outputs exactly `terminal_height` lines on each render; bubbletea's inline
renderer overwrites them in place, producing the "pinned bar" effect.

**Layout** (terminal_height = 24 example):

```
                                          ← empty
  Syncing 8 file(s) to host:/path        ← header
                                          ← empty
  ✓ src/main.go                          ┐
  ✓ src/config.go                        │  log area
  ✗ internal/bad.go                      │  (last N entries that fit)
  ...                                    ┘
──────────────────────────────────────── ← separator  (color 241)
  [=========>                 ]  5/8  62%  00:03       ← bar
──────────────────────────────────────── ← separator
```

Bar style: ASCII `[========>          ]` — filled with `=`, head `>` while
in progress, all `=` when complete. Filled portion color 116 (teal).

Info on bar: `N/Total  PCT%  MM:SS`. No current-file name (not requested).

Colors follow `context/ui-context.md` tokens:
- `✓` success: 82 (green)
- `✗` failure path: 196 (red)
- separator: 241 (dim)
- bar filled: 116 (teal)
- stats + header: 252 (light gray)

## Implementation

### `internal/tui/syncprogress.go` — new file

- **`SyncFileDone`** struct: `Path string`, `Err error` — sent by uploader goroutine.
- **`syncTickMsg`** type alias for `time.Time` — drives elapsed time re-render.
- **`SyncProgress`** model: `header string`, `done []SyncFileDone`, `total int`,
  `start time.Time`, `width int` (default 80), `height int` (default 24).
- `Init()` — returns `syncTick()` (200 ms tick).
- `Update()` — handles `WindowSizeMsg` (update width/height), `SyncFileDone`
  (append + quit when `len(done) == total`), `syncTickMsg` (re-tick),
  `ctrl+c` (quit).
- `View()` — builds the full-height view as described above. Log area height =
  `height - 6`. Pad top of log area with empty lines when fewer files are done.
  Last 3 lines: `sep \n barLine \n sep` (no trailing newline on last sep so
  bubbletea doesn't add an extra blank line).
- `renderBar()` — computes filled/empty widths, returns the styled bar string.
- `formatSyncDuration(d time.Duration) string` — returns `"MM:SS"`.
- **`RunSyncProgress(header string, files []string, upload func(string) error) (int, error)`** —
  creates model, creates `tea.NewProgram(model)`, starts goroutine that calls
  `upload(f)` per file and `p.Send(SyncFileDone{...})`, runs `p.Run()`,
  casts result back to `SyncProgress`, counts and returns failures.

### `cmd/sync.go` — update upload section

Remove the old `fmt.Printf` header, the `for _, f := range files` loop, and
the `failed` counter. Replace with a single call:

```go
failed, err := tui.RunSyncProgress(
    fmt.Sprintf("Syncing %d file(s) to %s:%s", len(changed), profile.Host, profile.Path),
    changed,
    func(localPath string) error {
        return client.UploadFile(localPath, filepath.Join(profile.Path, localPath))
    },
)
if err != nil {
    return err
}
if failed > 0 {
    return fmt.Errorf("%d file(s) failed to upload", failed)
}
```

Remove `iconSyncOK` and `iconSyncFail` constants if no longer used in sync.go
(they may be re-used inside syncprogress.go instead).

## Dependencies

- none (bubbletea v2, lipgloss v2 already in go.mod)

## Verify when done

- [ ] `teleport sync` with 3+ changed files shows the bar advancing file by file
- [ ] Bar reaches 100% and the final view shows all ✓/✗ entries
- [ ] Terminal returns to normal prompt after program exits (no leftover TUI)
- [ ] `teleport sync` with 0 changed files still prints "Nothing to sync" (no TUI)
- [ ] A failed upload shows ✗ in red alongside the path
- [ ] `go build ./...` passes with no errors
