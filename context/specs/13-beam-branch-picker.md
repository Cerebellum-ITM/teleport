# Unit 13: Beam branch picker — beam commits from any local branch

## Goal

Extend `teleport beam` to let the user pick which local branch to source
commits from, instead of always reading `@{u}..HEAD`. A new branch picker
TUI appears before the commit picker when the user does not pass `--branch`;
passing `--branch <name>` skips the TUI and sources commits directly.

## Design

### New flag

```
teleport beam [profile] [--branch <name>] [-b <name>]
```

- `--branch` / `-b` accepts any local branch name. When omitted, the
  existing flow is preserved **unless** there is no upstream on the current
  branch — in that case the branch picker opens automatically instead of
  aborting.
- The flag does not affect `--clean`, `--then-sync`, or `--yes`; those
  remain orthogonal.

### Branch picker TUI (new)

Shown before the commit picker when `--branch` is omitted. Lists all local
branches returned by `git branch --format=%(refname:short)`. The current
branch is pre-selected (cursor starts on it).

Visual style — same token vocabulary as the commit picker:
- Header: `  Select source branch` (reuse existing `headerStyle`)
- One branch per row: `▶ ` on cursor, branch name in bold, `(current)` dim
  suffix on the active branch.
- Keys: `↑/k`, `↓/j`, `enter` confirms, `ctrl+c` cancels.
- Single-select only — beam always sources from one branch.
- Footer: `↑↓=navigate  enter=confirm  ctrl+c=quit`
- API: `RunBranchPicker(branches []string, current string) (string, error)`

### Commit source logic

After a branch is resolved (via flag or TUI):

- If the selected branch has an upstream configured:
  use `git log <upstream>..<branch> --format=…` (replace `@{u}` with the
  explicit upstream ref obtained from
  `git rev-parse --abbrev-ref <branch>@{u}`).
- If the selected branch has **no** upstream:
  use `git log <branch> --not --remotes --format=…` to get all commits
  on the branch not reachable from any remote ref.
- If neither produces commits: print
  `Nothing to beam — no local commits on <branch> ahead of remote.` and
  exit 0.

The rest of the beam flow (file picker → upload → optional sync) is
unchanged.

### Error cases

- `--branch <name>` where `<name>` is not a local branch: exit with
  `error: branch "<name>" not found locally`.
- Branch picker cancelled (`ctrl+c`): exit 0 with no message (same as
  cancelling the commit picker today).

## Implementation

### `internal/git/git.go`

```go
// LocalBranches returns all local branch names, current branch first.
func LocalBranches() (current string, all []string, err error)
```

Implementation:
- Run `git branch --format=%(refname:short)` to get all branches.
- Run `git rev-parse --abbrev-ref HEAD` to identify current.
- Return current as first element of `all` as well (keeps the list
  complete for the picker).

```go
// CommitsAheadOf returns commits on branch that are not reachable from
// its upstream (if configured) or from any remote ref (if not).
// Returns ErrNoCommits (new sentinel) when the branch is fully in sync.
func CommitsAheadOf(branch string) ([]Commit, error)
```

- Check upstream: `git rev-parse --abbrev-ref <branch>@{u}` — if it
  succeeds, use `git log <upstream>..<branch> --format=…`.
- If it fails (no upstream): use
  `git log <branch> --not --remotes --format=…`.
- Reuses the same `--format=%H%x09%h%x09%s%x09%cr` as `CommitsAhead`.
- Returns `ErrNoCommits` (not `ErrNoUpstream`) when the list is empty —
  the caller decides whether that's an error or a normal "nothing to do".

Keep `CommitsAhead()` as a thin wrapper:
```go
func CommitsAhead() ([]Commit, error) {
    _, branches, err := LocalBranches()
    if err != nil || len(branches) == 0 {
        return nil, err
    }
    return CommitsAheadOf(branches[0]) // branches[0] is current
}
```

Add sentinel:
```go
var ErrNoCommits = errors.New("no commits ahead of remote")
```

### `internal/tui/branchpicker.go` — new file

Bubbletea v2 model, single-select. Keep it minimal — no filtering needed
(branch lists are short).

```go
type branchPickerModel struct {
    branches []string
    current  string
    cursor   int
    chosen   string
    quitting bool
}

func RunBranchPicker(branches []string, current string) (string, error)
```

Reuse `headerStyle`, `dimStyle`, `boldStyle` from the package. Do not
introduce new color tokens — the branch picker uses only structural styles.

### `cmd/beam.go`

1. Add flag:
   ```go
   var beamBranch string
   beamCmd.Flags().StringVarP(&beamBranch, "branch", "b", "", "source branch for commits (default: current branch)")
   ```

2. Replace the `git.CommitsAhead()` call block with new logic:

   ```go
   branch, err := resolveBranch(beamBranch)
   if err != nil {
       return err
   }

   commits, err := git.CommitsAheadOf(branch)
   if err != nil {
       return err
   }
   if len(commits) == 0 {
       if !beamClean {
           fmt.Printf("Nothing to beam — no local commits on %s ahead of remote.\n", branch)
       }
       return nil
   }
   ```

3. Add helper `resolveBranch` in `cmd/beam.go`:

   ```go
   // resolveBranch returns the branch to source commits from.
   // If explicit is non-empty, validates it exists locally.
   // Otherwise, opens the branch picker TUI.
   func resolveBranch(explicit string) (string, error) {
       current, all, err := git.LocalBranches()
       if err != nil {
           return "", fmt.Errorf("list branches: %w", err)
       }
       if explicit != "" {
           for _, b := range all {
               if b == explicit {
                   return b, nil
               }
           }
           return "", fmt.Errorf("branch %q not found locally", explicit)
       }
       if len(all) == 1 {
           return current, nil  // only one branch, skip TUI
       }
       return tui.RunBranchPicker(all, current)
   }
   ```

   Remove the now-unused `git.ErrNoUpstream` import guard from `runBeam`
   (it is handled inside `CommitsAheadOf`).

## Dependencies

No new packages.

## Verify when done

- [ ] `go build -o teleport . && go vet ./...` clean.
- [ ] `teleport beam` on a repo with multiple local branches: branch picker
  opens, current branch is pre-selected, enter proceeds to commit picker.
- [ ] `teleport beam` on a repo with only one local branch: branch picker
  is skipped, goes straight to commit picker.
- [ ] `teleport beam --branch feat/foo`: branch picker is skipped, commits
  from `feat/foo` are shown directly.
- [ ] `teleport beam --branch nonexistent`: exits with
  `error: branch "nonexistent" not found locally`, no TUI opens.
- [ ] Branch with upstream: commit picker shows only commits ahead of that
  upstream (same as current behaviour when on that branch).
- [ ] Branch without upstream: commit picker shows commits not reachable
  from any remote ref.
- [ ] Branch fully in sync (no commits ahead): prints
  `Nothing to beam — no local commits on <branch> ahead of remote.`
  and exits 0.
- [ ] Cancelling the branch picker with `ctrl+c` exits 0 without connecting
  to SSH.
- [ ] `CommitsAhead()` still works correctly (backwards compatibility for
  any future caller).
- [ ] `teleport beam --help` shows the new `--branch`/`-b` flag.
