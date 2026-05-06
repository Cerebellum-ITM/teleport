# Unit 01: Diff-based sync

## Goal

Replace the current "upload everything" sync with a git-status-aware sync that
uploads only files modified since the last commit. Add a `-u` / `--untracked`
flag to also include untracked files. Remove the file-picker TUI from the
normal sync flow.

## Design

No interactive TUI during sync. The command prints a list of files it detected
as changed, uploads them with per-file ✓/✗ feedback (same icons as today), and
exits. Fast, scriptable, no prompts.

If there are no changed files, print a short message and exit 0 cleanly.

## Implementation

### `internal/git` — add `ChangedFiles()`

Add a new function `ChangedFiles() ([]string, error)` that runs:

```
git diff --name-only HEAD
```

This returns all files that differ from HEAD — both staged and unstaged changes,
tracked files only. Parse the output exactly like `TrackedFiles()` does (split
on newlines, skip empty).

Do not remove `TrackedFiles()` or `UntrackedFiles()` — they may be used
elsewhere or in future units.

### `cmd/sync.go` — replace file picker with diff output

1. Remove the call to `tui.RunFilePicker(...)`.
2. Call `git.ChangedFiles()` to get modified tracked files.
3. If `--untracked` / `-u` flag is set, also call `git.UntrackedFiles()` and
   append those results.
4. Deduplicate the combined slice (a file could theoretically appear in both,
   though unlikely in practice).
5. If the final list is empty, print `"Nothing to sync — no changes since last
   commit."` and return nil.
6. Print a header before uploading:
   ```
   Syncing N file(s) to host:path
   ```
7. Upload files and print per-file ✓/✗ exactly as today.

### Flag wiring

Add the `-u` / `--untracked` flag to `syncCmd` in `cmd/sync.go`:

```go
syncCmd.Flags().BoolVarP(&includeUntracked, "untracked", "u", false, "also sync untracked files")
```

Declare `includeUntracked` as a package-level var in the same file.

## Dependencies

- none (uses only existing `internal/git` and standard library)

## Verify when done

- [ ] `teleport sync` with no local changes prints "Nothing to sync" and exits 0
- [ ] `teleport sync` with a modified tracked file uploads only that file, not all tracked files
- [ ] `teleport sync -u` also uploads untracked files alongside modified tracked files
- [ ] `teleport sync -u` with no changes and no untracked files prints "Nothing to sync"
- [ ] The file-picker TUI no longer appears during `teleport sync`
- [ ] `go build ./...` passes with no errors
- [ ] `git diff --name-only HEAD` returning an empty string is handled without panic
