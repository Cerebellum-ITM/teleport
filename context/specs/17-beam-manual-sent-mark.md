# Unit 17: Beam manual sent-mark — tell teleport the truth without re-beaming

## Goal

Let the operator manually mark/unmark commits as "already beamed" **from inside
the commit picker**, so the sent-tracking from [Unit 15](15-beam-sent-tracking.md)
can be corrected without actually re-sending anything. Identity in the beamed
store is the **exact commit SHA**, which means commits that are genuinely on the
remote can still show up as pending:

- a brand-new branch that never ran through `teleport beam` (no history),
- a rebased / amended / cherry-picked commit (same content, new SHA),
- the first beam from a different machine (the store is local).

This unit adds a live toggle (`m`) and a symmetric bulk action (`M`) in the
commit picker, accumulating the marks in the picker session and persisting the
delta to the per-profile beamed store **only on confirm (enter)**. Cancelling
(`ctrl+c`) discards every pending mark.

"Sent" stays **per destination profile** (Unit 15): marking for `staging` must
never touch `production`. The marks live in the same `BeamedCommits[profile]`
store the automatic tracking already uses — this unit adds no new storage.

## Design

### Concept: mark ≠ selection

teleport already overloads `tab`/space in the commit picker to mean *include
this commit in the beam* (what to upload now). The sent-mark is a **separate,
orthogonal** concept: *what is recorded as already applied on the remote*. The
new keys touch only the sent-mark (the dim/badge state), never the beam
selection. The two stay independent: marking a commit as sent does not
deselect it from the beam, and toggling its beam selection does not change its
sent-mark.

### Keys (commit picker, `internal/tui/commitpicker.go`)

The commit picker has **no text-filter input**, so plain letters are safe (no
clash). Reuses the project convention of single-letter keys.

- `m` — toggle the sent-mark of the commit **under the cursor**. The dim badge
  updates live. No-op when the list is empty. (Every row in this picker is a
  commit with a SHA, so there are no unmarkable rows — unlike a file picker
  with SHA-less "uncommitted changes" entries.)
- `M` (shift+m) — **bulk** toggle: if every commit is currently marked sent,
  unmark them all; otherwise mark them all sent. Symmetric. Operates on the
  full commit list (the picker has no filter, so all commits are "visible").
- Existing keys unchanged: `tab`/space (beam toggle), `a` (select-all beam),
  `u` (re-select unsent), `enter` (confirm), `ctrl+c` (cancel).

The dim/badge rendering from Unit 15 (`iconSent` `󰗠` + `sentStyle` green +
dimmed subject) is reused verbatim; `m`/`M` mutate the same `sent` map it reads,
so the visual updates with no extra rendering code.

### Live state & delta

- Snapshot the **original** sent set at construction time into a new field
  `origSent map[string]bool` (copy of the incoming `sent` map). The working
  `sent` map is what `m`/`M` mutate live.
- On confirm, the picker exposes the delta via a new method:

  ```go
  // SentDelta reports the manual sent-mark changes made during this picker
  // session, scoped to the commits actually shown. added = SHAs now marked
  // that were not in the original sent set; removed = SHAs unmarked that were.
  func (m CommitPicker) SentDelta() (added, removed []string)
  ```

  **Crucial bound:** `SentDelta` iterates only over `m.commits` (the SHAs the
  picker actually displayed). The beamed store may contain SHAs outside the
  visible commits-ahead window; those are **not** represented in the picker and
  must **never** be reported as `removed`. Iterating `m.commits` guarantees this
  — a SHA that isn't a picker row can never appear in either list.

- `RunCommitPicker` gains a second return value for the delta:

  ```go
  func RunCommitPicker(commits []git.Commit, sent map[string]bool) (selected []git.Commit, delta tui.SentMarkDelta, err error)
  ```

  where `SentMarkDelta` is a small struct `{ Added, Removed []string }`. On
  cancel (`ctrl+c`) it returns the existing `"cancelled"` error and an empty
  delta, so nothing is persisted. (Alternative considered: return three values
  `(selected, added, removed, err)` — rejected; a named struct reads better at
  the single call site.)

### Config helper (`internal/config/config.go`)

One new mutator that applies the whole delta in a single pass, then the caller
saves once (atomic write — `SaveLocal` truncates and re-encodes the file):

```go
// ApplyBeamedDelta marks every SHA in add as beamed to profile at time t and
// removes every SHA in remove. Creates the nested maps as needed and deletes
// the profile entry if it ends up empty. Mutator; caller persists via SaveLocal.
func (c *LocalConfig) ApplyBeamedDelta(profile string, add, remove []string, t time.Time)
```

Behaviour:
- `add`: same as `MarkBeamed` (ensure maps, set `sha = t`).
- `remove`: delete each SHA from `BeamedCommits[profile]`; tolerant of absent
  SHAs (no-op).
- If `BeamedCommits[profile]` becomes empty after removals, delete the profile
  key (consistent with `PruneBeamed`).
- No-op when both slices are empty.

`MarkBeamed` stays for the automatic post-upload path; `ApplyBeamedDelta` is the
add+remove superset used for the manual delta. (Not collapsing them: the
auto-path never removes, and keeping `MarkBeamed` avoids churn in Unit 15 code.)

### Beam orchestration (`cmd/beam.go`)

- Capture the delta from the picker and persist it **right after the picker
  confirms**, before the file picker / connection / upload — so the manual
  marks survive even if the user later cancels the file picker, a confirmation,
  or the upload fails:

  ```go
  selectedCommits, delta, err := tui.RunCommitPicker(commits, localCfg.SentSet(profileName))
  if err != nil { return err }
  if len(delta.Added) > 0 || len(delta.Removed) > 0 {
      localCfg.ApplyBeamedDelta(profileName, delta.Added, delta.Removed, time.Now())
      if err := config.SaveLocal(localCfg); err != nil {
          log.Warn("could not record manual sent marks", "err", err)
      }
  }
  if len(selectedCommits) == 0 { fmt.Println("No commits selected."); return nil }
  ```

  Persisting before the `len(selectedCommits) == 0` early-return is intentional:
  the operator may mark everything sent and then select nothing to beam — the
  marks must still stick.

- `localCfg` is already in memory and already pruned (`PruneBeamed`) before the
  picker, so the saved file reflects the pruned + delta state in one write.
- **Auto mode (`beam --auto`) is unchanged**: it skips the picker entirely, so
  there is no manual marking there (the keys are inert because there is no
  interactive selector). This satisfies the "non-interactive mode disables
  marking" requirement — `--auto` is teleport's non-interactive beam path.
- The later automatic `rememberBeamedCommits` (post-upload) is unchanged. A SHA
  the user manually marked and also actually beamed simply gets its timestamp
  refreshed — harmless.

### Help hint (footer)

Footer updated to announce the new keys while keeping the existing legend:

```
  tab=toggle  a=all  u=unsent  m=mark-sent  M=all-sent  enter=confirm  ctrl+c=quit
```

The Unit 15 visual legend (dim + green badge = already beamed) is conveyed by
the rendering itself and is unchanged.

### UI context (`context/ui-context.md`)

No new tokens or icons. The sent badge (`iconSent`, `sentStyle`) and `dimStyle`
already exist from Unit 15. Add a one-line note that `m`/`M` toggle the same
sent badge manually.

## Dependencies

- None. Reuses `BurntSushi/toml`, `time`, and the current bubbletea/lipgloss
  stack.

## Verify when done

- [ ] In the commit picker, `m` toggles the sent badge of the row under the
      cursor live, without changing its beam selection checkbox.
- [ ] `M` marks all commits sent in one press; pressing `M` again when all are
      marked unmarks them all (symmetric).
- [ ] Confirming with `enter` persists the delta to `BeamedCommits[profile]`
      even when no commits are selected to beam, and even if the file picker /
      upload is later cancelled.
- [ ] Cancelling with `ctrl+c` discards all pending marks (nothing written).
- [ ] Marking is add **and** remove: marking an unsent commit adds its SHA;
      unmarking a sent commit removes its SHA; both applied in a single
      `SaveLocal` write.
- [ ] Marks are per profile: marking for profile X does not affect profile Y.
- [ ] Unit test: `SentDelta` returns correct added/removed for the
      individual-toggle, bulk-toggle, and mixed cases, and **never** reports a
      removal for a SHA that is in history but not among the picker's commits.
- [ ] Unit test: `ApplyBeamedDelta` covers add, remove, mixed, absent-SHA
      removal (no-op), and empty-profile cleanup.
- [ ] `go build ./...` passes, `go vet ./...` is clean, and `go test ./...`
      passes (including existing TUI/config/beam tests).
