# Unit 28: `push` — upload arbitrary paths, git-agnostic

## Goal

Add `teleport push <local-path>... [profile]`: a transfer command that uploads the
paths it is given **exactly as given**, without consulting the git index or
`.gitignore`. It closes the only hole left in teleport's transfer story — a
gitignored build output (`web/dist`, `target/release`, `public/`) cannot be sent
today, and `sync -u` skips it *silently* while reporting success.

## Why it does not exist yet (the gap, with evidence)

| Command | Enumerates | Why it can't send a gitignored path |
| --- | --- | --- |
| `sync -u` | `git ls-files --others --exclude-standard` (`internal/git/git.go:37`) | `--exclude-standard` applies `.gitignore` → the path is **omitted in silence**; the command exits `0` and the remote keeps the stale copy. Worst possible failure mode. |
| `beam` | file contents at chosen commits | by definition only what git has |
| `mirror` | git objects via `git bundle` | by definition only what git has |
| `ship` | one arbitrary local file | validates the executable magic (`internal/bindetect`, `cmd/ship.go:76`) → `Error: detect binary type: not a recognised executable binary` for anything that isn't ELF/Mach-O/PE |

Motivating shape: a project that compiles a SPA to a gitignored `web/dist`. Deploying
it required leaving teleport for a hand-written `scp` in a `Makefile`, with teleport
left owning only the remote half (an action that extracts and verifies). `push`
replaces that `scp`.

**Non-goals, explicitly:** `ship`'s binary validation is *not* touched (a binary
channel should verify it gets a binary — the mistake was using it for non-binaries).
`sync` is *not* taught to include ignored files, not even behind a flag — it is the
git-aware command and its current semantics are correct and predictable. `push` is
the explicit escape hatch. No build detection, no toolchain logic, no stack-specific
behavior: this moves bytes.

## Design

### Name

**`push`**. `sync`/`beam`/`mirror`/`ship` are all send verbs with a specific
semantic; `push` is the universal term for "move these bytes there" and forms a
symmetric pair with the existing `pull` (both git-agnostic in their direction).
`send`/`put` are vaguer; `copy` is rejected outright — it reads as the mirror of
`pull` and would be confused with it.

### Shape

```sh
teleport push <local-path>... [profile] [--to <remote-subpath>] [--then <action>] [-y] [--dry-run] [--checksum]
```

Argument parsing follows `cmd/run.go`'s convention: the **last** arg is the profile
only when `len(args) >= 2` **and** it names an existing profile in the global config
**and** it does not exist as a local path. The local-path check is the tiebreaker
`run` doesn't need — a directory named like a profile must stay a source path.
`cobra.MinimumNArgs(1)`.

### Destination resolution — the profile is the boundary

`--to` is resolved **relative to the profile's `path`, never absolute**, and any
result that escapes the profile directory is rejected. A command that can write
anywhere on the server is a foot-trap; the profile stays the fence.

Local reference point is the **current working directory** (not the git root —
`push` must not call git at all, which is the whole point).

| Case | Destination |
| --- | --- |
| no `--to` | `<profile.path>/<path relative to cwd>` |
| `--to X`, **one** local path | `<profile.path>/X` — X is the exact destination path (file→file, dir→dir), so `push web/dist --to web/dist.new` renames the tree |
| `--to X`, **N** local paths | `<profile.path>/X/<basename(local)>` for each — X must be a directory |
| local path outside cwd (absolute, or `../…`) and no `--to` | error, exit `1`, naming the path and `--to` |

Rejections (all exit `1`, message names the offending value):

- `--to` that is absolute (`/etc`, or `C:\…`) → `--to must be relative to the profile path`
- any destination whose cleaned form escapes the profile path (`../`, `a/../../b`)
  → `destination escapes the profile path <path>`

Remote paths are POSIX: use `path.Join`/`path.Clean` for the remote side and
`filepath` only for local paths, so a Windows build doesn't emit `\` separators.
Containment test is `dest == base || strings.HasPrefix(dest, base+"/")` **after**
`path.Clean`, which also catches a `..` sneaking in via the default (no-`--to`) rel
path.

### Directories

Recursive by default — that is the use case (a `dist/`). No `-r` flag; there is no
sensible non-recursive meaning for "push this directory". Every directory in the
tree is created explicitly via `Client.MkdirAll` (including **empty** ones, so the
uploaded tree is faithful); file uploads also create their parents on their own.

### Deletion

**Not in v1.** No rsync-style `--delete`. Deleting on the remote is a different
class of risk and deserves its own unit and its own confirmation. The clean-replace
recipe is: push to a new directory and swap it in an action:

```sh
teleport push web/dist --to web/dist.new --then swap
```

### Symlinks

**Followed** (`os.Stat` semantics): a symlink to a file uploads the pointed-to file;
a symlink to a directory is walked. This matches `scp` and what someone pushing a
`dist/` expects. Never silent: a **broken** symlink is a loud error naming the path
(`push` aborts before uploading anything), and a **symlink cycle** is an error too
(detected with a `seen` set of `filepath.EvalSymlinks` results), not a silent skip.
Non-regular, non-directory entries (socket, fifo, device) are an error naming the
path.

Collection happens **entirely before** the first upload, so any of these failures
aborts with nothing sent.

### Permissions

Preserve the **execution bit only**: after upload, `chmod 0755` when the local file
has any exec bit set, `0644` otherwise. Not an unconditional `chmod +x` (that is
`ship`'s job), and not a raw copy of the local perm bits (umask artifacts and
meaningless Windows modes would leak through). One deterministic rule.

This is needed because `SFTP.Create` on an existing remote file keeps that file's
old mode — without the explicit chmod, a re-push could leave a stale mode in place.

### Verification

Size verification comes free: `UploadFileProgress` → `verifyUpload`
(`internal/ssh/client.go:344`, `:428`) already fails a truncated transfer
(invariant #6). `--checksum` additionally compares a local SHA256 against
`Client.RemoteSHA256` (`:441`) per file and fails the file on mismatch. Off by
default (it re-reads every byte on both ends); the result reports which level ran.

The summary states files, bytes, and the verification level.

### `--then`

Repeatable `--then/-t` via the existing `registerThenFlag` (`cmd/run.go:236`), same
as `sync`/`beam`/`mirror`: names validated against the profile **before** the
transfer starts, actions run only after a fully successful transfer over the same
connection, results nested as `actions[]` in the JSON, a failing action propagates
exit `1`. `-y` auto-confirms `confirm = true` actions, and its absence headless is
the `ErrNeedsTTY` (exit `2`) path — `push` opens no TUI of its own.

### `--dry-run`

Lists what it *would* upload (local → remote, per-file size, totals) and **never
connects**. `--then` names are still validated (config-only, no connection), but no
action runs. Cheap insurance.

### Not touched

`config.TouchLastSync()` is **not** called. `last sync` describes git parity with
the remote (`status`, `config get`); `push` says nothing about git, so stamping it
would misreport drift as freshness.

## Implementation

### `internal/ssh/client.go` — two thin methods

```go
// MkdirAll creates dir and any missing parents on the remote.
func (c *Client) MkdirAll(dir string) error {
    if err := c.SFTP.MkdirAll(dir); err != nil {
        return fmt.Errorf("mkdir remote %s: %w", dir, err)
    }
    return nil
}

// Chmod sets the mode of remotePath.
func (c *Client) Chmod(remotePath string, mode os.FileMode) error {
    if err := c.SFTP.Chmod(remotePath, mode); err != nil {
        return fmt.Errorf("chmod remote %s: %w", remotePath, err)
    }
    return nil
}
```

Placed next to `Mkdir` (`:298`). No new dependency.

### `cmd/push.go` — new file

Flags (registered in `init()`): `--to` (string), `--dry-run` (bool), `--checksum`
(bool), `-y/--yes` (bool), plus `registerThenFlag(pushCmd)`. `rootCmd.AddCommand(pushCmd)`.

Types:

```go
type pushItem struct {
    Local  string      // local path to read (symlinks resolved by stat)
    Remote string      // absolute remote destination
    Size   int64
    Mode   os.FileMode // 0755 if the local file has any exec bit, else 0644
}

type pushDir struct{ Remote string } // directories to create, parents first
```

Pure, testable destination helper:

```go
// resolvePushDest maps one local path to its absolute remote destination.
// base is the profile path, to is the raw --to value ("" = mirror the local
// path relative to cwd), single reports whether exactly one local path was
// given (which makes --to the exact destination rather than a directory).
func resolvePushDest(base, localPath, to string, single bool) (string, error)
```

Steps: reject absolute `to`; build the candidate with `path.Join`; `path.Clean`;
containment check against `base`; return. The no-`--to` branch computes
`filepath.Rel(cwd, abs(localPath))` and errors when the result starts with `..`
(hint: pass `--to`), converting separators with `filepath.ToSlash`.

Collection:

```go
// collectPushItems walks every local path, resolving symlinks and failing loudly
// on a missing path, broken symlink, cycle, or unsupported file type. Nothing is
// uploaded until this returns without error.
func collectPushItems(locals []string, base, to string) ([]pushItem, []pushDir, error)
```

`runPush` flow:

1. `resolveProfile`-style resolution with the trailing-profile rule above (reuse
   `config.LoadLocal`/`LoadGlobal`; a small `resolvePushProfile(args)` returning
   `(profile, name, locals, error)` keeps `runPush` thin).
2. `collectPushItems` → items, dirs. Empty item set with no dirs → error
   (a path that expands to nothing is a bug, not a no-op).
3. `validateThenActions` + `precheckActionsConfirm(profile, thenActions, pushYes || noInput)`.
4. If `--dry-run`: `emit(pushResult{DryRun: true, …}, human)` and return `nil`.
   No connection is opened.
5. `connectToProfile(profile)` (`cmd/clean.go:147`), `defer client.Close()`.
6. `client.MkdirAll` for each dir in `dirs` (parents first).
7. Upload via the existing progress plumbing — the display list is the local
   paths, the closure looks the destination up in a map:

   ```go
   header := fmt.Sprintf("Pushing %d file(s) (%s) to %s:%s", len(items),
       tui.HumanBytes(total), profile.Host, profile.Path)
   upload := func(local string) error {
       it := byLocal[local]
       if err := client.UploadFileProgress(it.Local, it.Remote, nil); err != nil {
           return err
       }
       if err := client.Chmod(it.Remote, it.Mode); err != nil {
           return err
       }
       if pushChecksum {
           return verifyChecksum(client, it)
       }
       return nil
   }
   if useTUI() { failed, err = tui.RunSyncProgress(header, locals, upload) } else {
       failed, err = tui.RunSyncPlain(header, locals, upload) }
   ```

   `verifyChecksum` hashes the local file with `crypto/sha256` and compares against
   `client.RemoteSHA256(it.Remote)`, erroring on mismatch.
8. `len(failed) > 0` → `fmt.Errorf("%d file(s) failed to upload", len(failed))`.
9. `executeActions(client, profile, name, thenActions, pushYes || noInput)`.
10. `emit(res, func(){})` (progress already printed inline), return the action error.

JSON shape:

```go
type pushFile struct {
    Local  string `json:"local"`
    Remote string `json:"remote"`
    Bytes  int64  `json:"bytes"`
}

type pushResult struct {
    Command  string         `json:"command"` // "push"
    Target   string         `json:"target"`  // host:path
    DryRun   bool           `json:"dry_run,omitempty"`
    Sent     int            `json:"sent"`
    Bytes    int64          `json:"bytes"`
    Verified string         `json:"verified"` // "size" | "checksum" | "" (dry-run)
    Files    []pushFile     `json:"files"`
    Actions  []actionResult `json:"actions,omitempty"`
}
```

All decoration (header, per-file lines, dry-run plan) goes to **stderr** under
`--json` — `RunSyncPlain` already writes to stderr, and the dry-run plan must use
`emit`'s human branch so stdout stays a single object.

### `cmd/root.go`, `cmd/help.go`

`rootCmd.AddCommand(pushCmd)`; a `push` row and one example in the help doc
(`` push  upload paths git ignores (builds, artifacts) ``). No root shorthand flag —
`push` requires arguments, so a bare `-P` has nothing to act on.

### Tests — `cmd/push_test.go`

`TestResolvePushDest` (table-driven), base `/srv/app`:

| local | to | single | want |
| --- | --- | --- | --- |
| `web/dist` | `""` | true | `/srv/app/web/dist` |
| `web/dist` | `web/dist.new` | true | `/srv/app/web/dist.new` |
| `web/dist` | `.` | true | `/srv/app` |
| `a.txt` | `releases` | false | `/srv/app/releases/a.txt` |
| `web/dist` | `../etc` | true | error (escape) |
| `web/dist` | `a/../../b` | true | error (escape) |
| `web/dist` | `/etc` | true | error (absolute) |
| `web/dist` | `sub/` | true | `/srv/app/sub` (trailing slash cleaned) |

`TestCollectPushItems` with `t.Chdir(t.TempDir())` and a fixture tree: nested dirs,
a file whose name would be gitignored, a symlink to a file (followed → uploads the
target's content, exec bit preserved), an empty dir (yields a `pushDir`), plus error
cases — missing path, broken symlink, symlink cycle — each asserted to return an
error naming the path and to collect **nothing**.

### Docs

- `README.md`: new `## Push — upload anything, git or not` section after `## Ship`,
  with a table that says **explicitly** how it differs from the neighbours (this is
  the exact spot people get it wrong, so it goes in writing):

  | | sees `.gitignore` | accepts | remote git history |
  | --- | --- | --- | --- |
  | `sync` | **yes** — ignored paths are skipped silently | git-tracked (+ `-u` untracked, still not ignored) | untouched |
  | `push` | **no** — uploads exactly what you name | any file or directory | untouched |
  | `ship` | n/a | **executables only** (ELF/Mach-O/PE) | untouched |

  Also update `## Three ways to ship code` → four, the `## Commands` table, and the
  `## Scripting (headless)` examples.
- `CHANGELOG.md`: `[Unreleased] → Added`.
- `context/architecture.md`: `push` in System Boundaries commentary + the
  destination-containment rule as an invariant ("`push` never writes outside the
  resolved profile path").
- `context/project-overview.md`: Features → File Sync entry; Scope → in-scope.
- `context/progress-tracker.md`: Unit 28 completed + session note.

## Dependencies

None. Everything reuses `internal/ssh` (`UploadFileProgress`, `verifyUpload`,
`RemoteSHA256`, `Mkdir`, `ShellQuote`), `internal/tui` (`RunSyncProgress`/
`RunSyncPlain`, `HumanBytes`), and `cmd`'s existing headless/action plumbing.
`crypto/sha256` is stdlib.

## Verify when done

- [ ] `gofmt -l cmd internal`, `go vet ./...`, `go test ./...`, `go build -o teleport .` all clean.
- [ ] `teleport push <gitignored-file>` uploads it; an unwritable destination fails
      loudly (non-zero exit, error names the path); a nonexistent path is an error,
      not a no-op.
- [ ] `teleport push <dir>` uploads the whole tree, creating intermediate (and
      empty) directories.
- [ ] `--to ../escape` and `--to /abs` are both rejected with a clear error, and
      both cases are covered by `TestResolvePushDest`.
- [ ] `--json` prints exactly one parseable object on stdout with `files[]`, `sent`,
      `bytes`, and `verified`; no decoration on stdout (`teleport push … --json | jq .`).
- [ ] Headless with a `confirm = true` `--then` action and no `-y` exits `2` and the
      message names `-y`; nothing is uploaded.
- [ ] `--then <action>` runs only after a successful transfer and nests its result
      as `actions[]`, matching `sync`/`mirror`.
- [ ] `--dry-run` opens no connection (verifiable with a profile pointing at an
      unreachable host: it must still print the plan and exit `0`) and changes nothing.
- [ ] A file with the local exec bit lands `0755` on the remote; a plain file lands
      `0644`; a broken symlink aborts the run before any upload.
- [ ] **Real SFTP smoke test** against a reachable host, in a throwaway remote
      directory, after the user OKs the remote write: push a tree containing a
      nested directory, an empty directory and an executable file, then confirm over
      SSH that every file arrived byte-identical (`sha256sum`) with the expected
      modes, and remove the directory afterwards. Repeat with `--to` renaming the
      tree and with `--then` chaining an action.
- [ ] `README.md` documents `push` **and** states in writing how it differs from
      `sync` (git-aware, skips ignored files) and `ship` (executables only).
- [ ] `CHANGELOG.md` and `context/progress-tracker.md` updated; one commit via
      `commitcraft` (English message, Spanish keypoints, no AI co-author).
