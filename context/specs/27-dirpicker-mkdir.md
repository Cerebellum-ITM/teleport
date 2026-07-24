# Unit 27: Create the destination folder named after the local project

## Goal

In the remote directory browser (`internal/tui/dirpicker.go`), add a key that
creates a new remote subfolder **named exactly like the local project** inside the
directory currently shown, then descends into it so `enter` confirms it. This lets
`teleport init` set up a remote that mirrors the local project directory name in
one keystroke instead of pre-creating the folder over SSH by hand.

## Design

### Trigger point

A key in the dir picker (the interactive browser used by `init` for both the sync
directory and bin directories). Not an `init`-only prompt — putting it in the
picker makes it reusable across every dir-selection flow.

### The folder name

Fixed to the **local project directory basename** (`filepath.Base(cwd)`),
computed by the caller and passed into the picker as `newDirName`. The picker
stays generic: when `newDirName == ""` the create key is inert and hidden from the
footer (so `cmd/actions.go`'s working-directory picker, which has no project name
to offer, is unaffected).

### Key binding

`ctrl+n` ("new"). A plain letter (`n`, `+`, `m`) is unusable because every
unhandled key is forwarded to the always-focused filter `textinput` — it would be
typed into the filter instead of creating a folder. `ctrl+n` is not a printable
filter character, so it is safe. Footer gains `ctrl+n=new <name>` only when
`newDirName != ""`.

### Flow on press

1. `target := filepath.Join(m.cwd, newDirName)`.
2. Run an SFTP mkdir command (async `tea.Cmd`, mirroring the existing
   `listDirsCmd` I/O pattern the picker already uses).
3. On success (or if the folder already exists — treated as success/idempotent):
   set `m.cwd = target`, reload its listing, so the header/`Selected:` line shows
   the new path and `enter` confirms it immediately.
4. On error: surface it via the existing `m.err` render path (non-fatal; the user
   can keep browsing).

Idempotent by design: pressing `ctrl+n` when the folder already exists just
descends into it rather than erroring — the user gets the same end state.

### Styling

Reuse existing `dimStyle` for the footer hint; no new theme tokens. The footer
string is built conditionally so widths stay tidy when the key is hidden.

## Implementation

### `internal/ssh/client.go` — new method

Add a single-purpose remote mkdir next to `ListDirs`:

```go
// Mkdir creates dir on the remote. It is idempotent: an already-existing
// directory is not an error. Parent directories must already exist.
func (c *Client) Mkdir(dir string) error {
    if err := c.SFTP.Mkdir(dir); err != nil {
        // Tolerate "already exists": re-stat and accept if it is a dir.
        if info, statErr := c.SFTP.Stat(dir); statErr == nil && info.IsDir() {
            return nil
        }
        return fmt.Errorf("mkdir remote %s: %w", dir, err)
    }
    return nil
}
```

(`filepath.Base(cwd)` is a single new segment, so `Mkdir` — not `MkdirAll` — is
correct; the parent is the currently-browsed dir, which exists.)

### `internal/tui/dirpicker.go`

- Add field `newDirName string` to `DirPicker`.
- New message types + command, mirroring `listDirsCmd`:

  ```go
  type mkdirDoneMsg struct{ path string }
  type mkdirErrMsg struct{ err error }

  func mkdirCmd(client *sshpkg.Client, path string) tea.Cmd {
      return func() tea.Msg {
          if err := client.Mkdir(path); err != nil {
              return mkdirErrMsg{err}
          }
          return mkdirDoneMsg{path}
      }
  }
  ```

- Handle the new messages in `Update`:
  - `mkdirDoneMsg`: `m.cwd = msg.path; m.loading = true; m.cursor = 0;
    m.filter.SetValue(""); return m, listDirsCmd(m.client, m.cwd)`.
  - `mkdirErrMsg`: `m.err = msg.err; return m, nil`.
- Add the key case in the `tea.KeyPressMsg` switch (before the filter fallthrough):

  ```go
  case "ctrl+n":
      if m.newDirName != "" {
          target := filepath.Join(m.cwd, m.newDirName)
          m.loading = true
          return m, mkdirCmd(m.client, target)
      }
      return m, nil
  ```

- Footer (`View`): append `  ctrl+n=new <newDirName>` to the help line only when
  `m.newDirName != ""`.

### Constructor / runner threading

Keep the existing signatures working (3 call sites) and add a name-aware variant:

- Add `newDirName` to the internal struct via a small builder to avoid a breaking
  signature change:

  ```go
  func (m DirPicker) WithNewDir(name string) DirPicker { m.newDirName = name; return m }

  func RunDirPickerNew(client *sshpkg.Client, startPath, header, newDirName string) (string, error) {
      p := tea.NewProgram(NewDirPickerWith(client, startPath, header).WithNewDir(newDirName))
      // ...same run/extract body as RunDirPickerWith...
  }
  ```

  Refactor `RunDirPickerWith` to delegate to `RunDirPickerNew(client, startPath,
  header, "")` so there is one run body.

### `cmd/init.go`

Compute the project name once and pass it to the two sync/bin pickers:

```go
projectName := ""
if cwd, err := os.Getwd(); err == nil {
    projectName = filepath.Base(cwd)
}
// sync:
remotePath, err := tui.RunDirPickerNew(client, "/", "  Select sync directory", projectName)
// bin:
binPath, err := tui.RunDirPickerNew(client, startPath, header, projectName)
```

`cmd/actions.go` keeps calling `RunDirPickerWith` (no project folder concept there
→ key stays inert).

## Dependencies

None (uses the existing `github.com/pkg/sftp` client already wired in
`internal/ssh`; no new third-party packages).

## Verify when done

- [ ] `go build -o teleport . && go vet ./... && gofmt -l cmd/ internal/` clean;
  `go test ./...` passes.
- [ ] In `teleport init` (sync), browsing to a parent and pressing `ctrl+n`
  creates `<parent>/<local-project-name>` on the remote and lands the picker
  inside it, with `Selected:` showing the new path; `enter` saves that path.
- [ ] Pressing `ctrl+n` when the folder already exists descends into it without
  error (idempotent).
- [ ] The footer shows `ctrl+n=new <name>` in `init`'s pickers and **hides** it in
  `teleport actions add`'s working-directory picker (no `newDirName`).
- [ ] A mkdir failure (e.g. no write permission on the parent) shows the error in
  the picker and lets the user keep browsing (non-fatal).
- [ ] Existing dir-picker behavior (filter, tab/→ descend, shift+tab/← up,
  enter=select, q/esc) is unchanged.
- [ ] `RunDirPickerWith` callers (`cmd/actions.go`, and the delegating path)
  compile and behave as before.
