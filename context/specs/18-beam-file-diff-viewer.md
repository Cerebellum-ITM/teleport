# Unit 18: Beam file & diff viewer — read the blob before you beam it

## Goal

In the beam file picker (`internal/tui/beamfilepicker.go`, the
"Files from selected commits" screen) let the operator inspect the file under
the cursor **without leaving the picker**: press `v` to view the full file
contents and `d` to view the diff that commit introduced, both rendered in a
`bat`-style pager with syntax highlighting and line numbers. Inside the pager
`tab` switches file ⇄ diff, `esc`/`q` returns to the picker with the cursor and
the beam selection intact.

Because beam uploads **committed blobs**, not the working tree, "the file" is
the blob at that file's commit SHA (`git show <sha>:<path>`, the existing
[`git.FileAtCommit`](../../internal/git/git.go)) and "the diff" is the change
that *that* commit introduced for *that* path (`git show <sha> -- <path>`, a new
helper). Nothing here reads the working tree or touches the remote.

## Design

### Concept: view ≠ selection

The picker already overloads `tab`/space to mean *include this file in the
beam*. Viewing is **orthogonal**, like the sent-mark in
[Unit 17](17-beam-manual-sent-mark.md): `v`/`d` open a read-only pager and
change nothing about the selection. Returning from the pager restores the exact
picker state (cursor row, filter, checkboxes).

### Two new keys (file picker)

The beam file picker has **no text-filter input**, so plain letters are safe.

- `v` — open the pager in **file** mode for the file under the cursor.
- `d` — open the pager in **diff** mode for the file under the cursor.
- Existing keys unchanged: `↑/k ↓/j` (move), `←/h →/l` (commit filter),
  `tab`/space (beam toggle), `a` (toggle filter group), `enter` (confirm),
  `ctrl+c` (cancel).

### The pager (sub-model, not a separate program)

The viewer is a **sub-model embedded in the picker**, not a separate
`tea.Program`. The picker gains a `viewing bool` + a `*beamFileViewer` field;
while `viewing` is true the picker delegates `Update`/`View` to the viewer and
ignores its own key handling. This keeps one TUI session (no alternate-screen
flicker between screens) and trivially preserves picker state on return.

`beamFileViewer` (new file `internal/tui/beamfileviewer.go`) wraps
[`bubbles/v2/viewport`](https://charm.land) for scrolling — the first use of
viewport in the repo; add it from the already-present `charm.land/bubbles/v2`
module (no new top-level dependency for the component itself).

State:

```go
type viewerMode int
const ( viewerFile viewerMode = iota; viewerDiff )

type beamFileViewer struct {
    fc       git.FileChange     // path + status + SHA of the file
    short    string             // short SHA for the header (from the legend)
    mode     viewerMode
    vp        viewport.Model
    width, height int
    // rendered content cached per mode so tab/re-open is instant
    rendered map[viewerMode]string
    err      error              // load/highlight error, shown inline
}
```

Header bar (`bat`-style, one line, reusing existing styles):

- file mode: `{fileTypeIcon} {path}  ·  {lang}  ·  󰜘 {short}  ·  {N} líneas`
- diff mode: `󰦓 {path}  ·  diff  ·  󰜘 {short}  ·  +{adds} −{dels}`

`fileTypeIcon(path)` already exists in `internal/tui/syncprogress.go`; reuse it.
`{lang}` is chroma's lexer name (see highlight package). Header uses
`headerStyle`; the metadata segments after `·` use `dimStyle`; the short SHA is
tinted with the commit's accent (`m.shaStyle[fc.SHA]` passed in from the picker,
the same `beamCommitColors` source already used).

Footer (one line, `dimStyle`, the `b` highlight follows the `m/M` footer
precedent from Unit 17):

```
  j/k ↑/↓=scroll  ^d/^u=½ página  g/G=inicio/fin  tab=ver diff  esc=volver  ctrl+c=quit
```

`tab=ver diff` flips to `tab=ver archivo` in diff mode.

Keys inside the pager:

- `j`/`k`/`↑`/`↓` — line scroll (viewport).
- `ctrl+d`/`ctrl+u` — half-page (viewport `HalfPageDown/Up`).
- `g`/`G` — top / bottom (`GotoTop`/`GotoBottom`).
- `tab` — toggle `viewerFile` ⇄ `viewerDiff`; lazily loads the other mode on
  first switch and caches it.
- `esc`/`q` — close the pager, hand control back to the picker (`viewing=false`).
- `ctrl+c` — quit the whole flow (sets the picker's `quitting`, like everywhere).

### Syntax highlighting (`internal/highlight`, new package)

New `internal/highlight` wraps [chroma v2](https://github.com/alecthomas/chroma).
chroma is **pure Go, no CGO** → respects architecture invariant #5 (static
binary). `charm.land/ultraviolet` is only Charm's cell buffer, not a code
highlighter, so it does not cover this.

```go
// Code highlights src as the language inferred from filename, emitting ANSI for
// the given color profile, with a left line-number gutter (bat-style).
func Code(src []byte, filename string, p colorprofile.Profile) (out string, lang string, lines int, err error)

// Diff colorizes a unified-diff blob (output of `git show … -- path`): '+' lines
// green, '-' lines red, '@@' hunk headers on a dim bar, context dim. Returns the
// rendered text and the +/- line counts for the header.
func Diff(raw []byte, p colorprofile.Profile) (out string, adds, dels int)
```

Decisions:

- **Lexer**: `lexers.Match(filename)`, fallback `lexers.Analyse(string(src))`,
  final fallback `lexers.Fallback` (plain). `lang` returned for the header.
- **Formatter**: pick by `colorprofile.Profile` — `TrueColor` →
  `formatters.TTY16m`, else `formatters.TTY256`; `Ascii`/no-color → the plain
  `NoOp` formatter (gutter only, no ANSI). The project already threads
  `colorprofile` (see `architecture.md`), so the profile is available; if a
  profile isn't easily reachable from the picker, default to `TTY256` (safe on
  modern terminals) and note it.
- **Style**: a dark style consistent with the "dark technical workspace"
  (`ui-context.md`). `catppuccin/go` is already a dependency, so build a chroma
  style from Catppuccin Mocha for visual coherence; if that's more than this
  unit needs, use chroma's built-in `"catppuccin-mocha"` style token. Either way
  the gutter line-number color maps to `dimStyle` (`241`).
- **Gutter**: right-aligned line numbers in `dimStyle` + ` │ ` separator, like
  the mockup. Width = digits of the last line number.
- **Diff coloring (delta-style)**: `highlight.Diff(raw, filename, width, profile)`
  syntax-highlights the code on each line (per-line `lineHighlighter`) and
  prepends a tinted two-column (old/new) line-number gutter: added = green chip
  (fg `83` / bg `22`), removed = red chip (fg `210` / bg `52`), context dim
  `241`. Hunk starting line numbers are parsed from `@@ -a,b +c,d @@` to drive
  the counters. Per-line highlighting does not preserve multi-line lexer state
  (block comments, raw strings) but is robust; a tokenise failure returns the
  line uncolored rather than breaking the render.
- **Hunk bar + hidden file header**: each hunk opens with a **full-width bar**
  (`hunkBar`, bg `238` padded to `width`) showing the `@@ -a,b +c,d @@` range
  dim (`245`) and the section context (enclosing function/class) in accent
  (`117`), preceded by a blank separator line between hunks. The file-header
  block (`diff --git`/`index`/`---`/`+++`) is **dropped** — the viewer header
  already shows the path and SHA. `width` reaches `Diff` through
  `FileContentFunc(fc, mode, width)` (the picker passes `m.width`, captured when
  the viewer opens); resize does not re-flow the bar (acceptable).

### I/O stays in the caller (invariant #3)

TUI models must not perform I/O. The picker therefore receives a **content
provider** closure, defined in `cmd/beam.go` (which already imports `git`):

```go
type FileContentFunc func(fc git.FileChange, mode tui.ViewerMode) (string, error)
```

Flow: on `v`/`d` the picker emits a `tea.Cmd` that calls the provider →
provider does git + highlight off the UI thread → returns a
`viewerContentMsg{mode, content, err}` the picker feeds into the viewer.
Content is **loaded lazily** (only when opened) and cached in
`beamFileViewer.rendered`, so re-opening or `tab` is instant. A spinner/“cargando…”
line is shown until the first message arrives (files can be large).

`RunBeamFilePicker` gains the provider parameter:

```go
func RunBeamFilePicker(changes []git.FileChange, commits []git.Commit, load FileContentFunc) ([]git.FileChange, error)
```

The single call site (`cmd/beam.go:153`) passes a closure that switches on
`mode`:

- `viewerFile`: bytes from `git.FileAtCommit(fc.SHA, fc.Path)`; for a deleted
  file (`fc.Status == 'D'`) use the pre-delete blob `git show <sha>^:<path>`
  (new `git.FileBeforeCommit`), header notes "(antes de borrar)". Then
  `highlight.Code(...)`.
- `viewerDiff`: bytes from new `git.FileDiffAtCommit(fc.SHA, fc.Path)`; then
  `highlight.Diff(...)`.

### New git helpers (`internal/git/git.go`)

```go
// FileDiffAtCommit returns the unified diff a commit introduced for one path.
func FileDiffAtCommit(sha, path string) ([]byte, error) // git show --format= <sha> -- <path>

// FileBeforeCommit returns the blob of path as it was in the commit's first
// parent (used to view a file that this commit deleted).
func FileBeforeCommit(sha, path string) ([]byte, error) // git show <sha>^:<path>
```

Both mirror `FileAtCommit`'s shape (run git, wrap error). Renames: `FileChange`
already splits a rename into a `'D'` (old path) + `'A'` (new path), so each side
views/diffs its own path with no special handling here.

### Edge cases

- **Deleted file (`'D'`)**: file mode shows the pre-delete blob via
  `FileBeforeCommit`; diff mode shows the deletion hunk. Header marks it.
- **Binary blob**: detect a NUL byte in the first ~8 KB; skip chroma and render
  a placeholder `「binary file · N bytes」` in `dimStyle` (no scroll content).
- **Large file**: viewport paginates fine; cap chroma tokenizing at a generous
  line ceiling (e.g. 5000) and render the remainder un-highlighted rather than
  truncating, with a dim note. Not truncated — just not colored past the cap.
- **Empty diff / empty file**: render an explicit dim `「sin contenido」` line.

### UI context (`context/ui-context.md`)

- Keybinding conventions: note that in the beam file picker `v` opens the file
  viewer and `d` the diff viewer (read-only, orthogonal to `tab` selection),
  and that the viewer is the one place `tab` means *switch file⇄diff*.
- New icons: `iconEye 󰈈` (reserved, optional in header) and `iconDiff 󰦓`
  (diff-mode header). Document them in the icon table alongside the existing
  beam icons.
- New "File / Diff Viewer" entry under Component Patterns describing the
  `bat`-style header + line-number gutter, the green/red diff coloring (reusing
  `checkStyle` `82` / `deleteStyle` `203` / `dimStyle` `241`), and the
  Catppuccin-Mocha chroma style.

## Dependencies

- `github.com/alecthomas/chroma/v2` — syntax highlighting + diff lexer; pure Go,
  no CGO (satisfies invariant #5).
- `charm.land/bubbles/v2/viewport` — scrollable pager; the module is already a
  dependency, this only newly imports the `viewport` sub-package.
- (Catppuccin palette reused from the existing `github.com/catppuccin/go` dep;
  no new top-level dependency for the style.)

## Verify when done

- [ ] In the beam file picker, `v` opens a full-file pager for the file under
      the cursor with syntax highlighting and a left line-number gutter; `esc`
      returns to the picker with the same cursor row, commit filter, and
      checkbox state.
- [ ] `d` opens the unified diff that the file's commit introduced, with `+`
      lines green, `-` lines red, hunk headers on a dim bar, context dim, and a
      `+adds −dels` count in the header.
- [ ] Inside the pager `tab` switches file ⇄ diff without re-fetching already
      loaded content (cached), and the footer hint flips accordingly.
- [ ] Scrolling works: `j/k`, `↑/↓`, `ctrl+d/ctrl+u`, `g/G`; large files
      paginate without breaking layout.
- [ ] Opening `v`/`d` never changes the beam selection; confirming with `enter`
      after viewing uploads exactly the files that were checked.
- [ ] A deleted file (`'D'`) shows its pre-delete contents in file mode and the
      deletion hunk in diff mode; a binary blob shows the `「binary file」`
      placeholder instead of garbage.
- [ ] All git/highlight I/O happens in the `cmd/beam.go` provider closure, not
      inside the TUI model (invariant #3); the binary still builds with no CGO
      (invariant #5).
- [ ] Unit test: `git.FileDiffAtCommit` / `git.FileBeforeCommit` return the
      expected output for an added, modified, and deleted path in a throwaway
      repo.
- [ ] Unit test: `highlight.Diff` reports correct `adds`/`dels` and colors `+`/
      `-`/context/hunk lines distinctly; `highlight.Code` returns a non-empty
      `lang` for a known extension and falls back cleanly for an unknown one.
- [ ] `go build ./...` passes, `go vet ./...` is clean, `gofmt` clean, and
      `go test ./...` passes (including existing TUI/git tests).
