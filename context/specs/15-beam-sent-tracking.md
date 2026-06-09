# Unit 15: Beam sent-commit tracking — remember what was already beamed

## Goal

Make `teleport beam` remember which commits have already been beamed **to a
given profile**, surface that state visually in the commit picker, pre-select
only the not-yet-sent commits when the picker opens, and add a `u` key to
re-select the unsent set on demand. The state is persisted per project and per
profile in the local config and auto-pruned so it never grows unbounded.

"Sent" is always **relative to a destination profile**: a commit beamed to
`production` is not considered sent for `staging`. The state is therefore keyed
by the profile name resolved by `resolveProfile`.

## Design

### State model & persistence

New field on `LocalConfig` (`internal/config/config.go`), persisted in the
existing per-project local config (`~/.config/teleport/projects/<hash>.toml`):

```go
// BeamedCommits maps a profile name to the set of commit SHAs already beamed
// to that destination, with the time each was sent.
BeamedCommits map[string]map[string]time.Time `toml:"beamed_commits,omitempty"`
```

TOML representation (full 40-char SHAs are valid bare keys):

```toml
[beamed_commits.production]
a1b2c3d4e5f6... = 2026-06-09T18:30:00Z
0a9b8c7d6e5f... = 2026-06-09T18:30:00Z
```

**Auto-prune.** At the start of every `beam`, after computing the
commits-ahead list, drop from `BeamedCommits[profile]` any SHA that is no
longer in that list (it was pushed/merged, or rewritten by rebase/amend). This
keeps the file bounded and makes rebased/amended commits reappear as unsent
(correct — the content changed, the SHA is new).

**Marking granularity (per-file OK).** A commit is recorded as sent iff it has
**at least one file in the final beam selection** AND **all of its files in
that selection (uploads and deletes) completed without error**. A commit whose
files were all deselected in the file picker is not marked (nothing was sent
for it); a commit with any failed file is not marked (so it reappears next
run). The recorded timestamp is the time of the beam.

### Commit picker changes (`internal/tui/commitpicker.go`)

- New field `sent map[string]bool`; constructor becomes
  `NewCommitPicker(commits []git.Commit, sent map[string]bool)`.
- **Pre-selection:** on construction set `selected[i] = !sent[c.SHA]` so the
  picker opens with exactly the unsent commits checked. (Today it opens with
  nothing checked.)
- **Visual marker (badge + dim subject):** a fixed-width badge column between
  the selection checkbox and the short SHA. Sent commits show `iconSent`
  rendered with `sentStyle` (success green, `lipgloss.Color("82")` — see
  `ui-context.md`); unsent commits render blank spaces of the same width to
  keep columns aligned. The subject of a sent commit is rendered with
  `dimStyle`; unsent subjects keep the normal style. The colored short SHA and
  relative date are unchanged.

  Row layout (sent vs unsent):

  ```
    ▶ 󰱒 󰗠 a1b2c3  feat: add ship command      hace 2h     (sent: badge + dim subject)
      󰄱    d4e5f6  fix: parser edge case        hace 3h     (unsent: blank badge, normal subject)
  ```

- **Keys:** keep `tab`/space toggle and `a` (toggle all). Add `u` =
  re-select exactly the unsent set (`selected[i] = !sent[c.SHA]` for all `i`).
  Footer updated to:
  `tab=toggle  a=all  u=unsent  enter=confirm  ctrl+c=quit`.
- The sliding-window rendering (Unit's prior list-overflow fix) is unchanged;
  the badge adds no rows.
- **API:** `RunCommitPicker(commits []git.Commit, sent map[string]bool) ([]git.Commit, error)`.
  `RunCommitPicker` is only called from `cmd/beam.go`, so the signature change
  is contained.

### Upload result reporting (`internal/tui/syncprogress.go`)

`RunSyncProgress` currently returns `(failed int, err error)`. Change it to
return the **failed paths** so beam can attribute failures to commits:

```go
func RunSyncProgress(header string, files []string, upload func(string) error) ([]string, error)
```

Returns the slice of paths whose upload returned an error. Update the two other
call sites to use `len(failed)`:
- `cmd/sync.go` (main sync)
- `cmd/beam.go` `runChainedSync` (the `--then-sync` stage)

### Beam orchestration (`cmd/beam.go`)

- Capture the profile name: `profile, profileName, err := resolveProfile(args)`
  (currently discards it as `_`).
- After `commits, err := git.CommitsAheadOf(branch)`:
  - Prune `localCfg.BeamedCommits[profileName]` to the set of ahead SHAs
    (in-memory; persisted at the end).
  - Build `sent := localCfg.SentSet(profileName)` and pass it to
    `RunCommitPicker(commits, sent)`.
- After the upload and delete phases, collect the set of failed paths
  (`failedPaths` from `RunSyncProgress` for uploads, plus paths whose
  `client.Remove` errored for deletes).
- Compute the newly-sent commits: for each distinct SHA present in the final
  `changes`, mark it sent iff none of its `changes` paths is in `failedPaths`.
  Record those SHAs in `localCfg.BeamedCommits[profileName]` with `time.Now()`
  and `config.SaveLocal(localCfg)`.
- **Ordering:** perform the marking + save **before** returning the
  `"%d operation(s) failed"` error, so commits that fully succeeded are
  recorded even on a partial failure. A `SaveLocal` failure is warn-logged, not
  fatal (same treatment as `TouchLastSync`).

### Config helpers (`internal/config/config.go`)

- `func (c *LocalConfig) SentSet(profile string) map[string]bool` — membership
  view of `BeamedCommits[profile]` (empty map when absent).
- `func (c *LocalConfig) PruneBeamed(profile string, keep map[string]bool)` —
  drop SHAs not in `keep` (mutator; caller saves).
- `func (c *LocalConfig) MarkBeamed(profile string, shas []string, t time.Time)` —
  ensure the nested maps exist and set each SHA to `t` (mutator; caller saves).

### UI context (`context/ui-context.md`)

- Add `iconSent` to the Nerd Font icon table (proposed glyph `󰗠 `
  md-check-circle; final glyph picked via the `nerd-fonts` skill at
  implementation time) — "commit already beamed to the active profile".
- Note the sent badge reuses the success-green token `lipgloss.Color("82")`.

### Out of scope (possible future units)

- `teleport beam --reset-sent` (or a `config`-style command) to clear a
  profile's beamed history.
- A manual `m` key in the picker to toggle a commit's sent state without
  actually sending it.

## Dependencies

- None. Uses the existing `BurntSushi/toml`, `time`, and the current TUI stack.

## Verify when done

- [ ] After beaming a subset of commits to profile X, re-running `teleport beam X`
      shows those commits with the sent badge, dimmed subject, and pre-selected
      OFF; the remaining commits are pre-selected ON.
- [ ] Pressing `u` in the commit picker re-selects exactly the unsent commits.
- [ ] A commit whose files all failed to upload is NOT marked as sent and
      reappears unsent on the next run.
- [ ] A commit fully succeeds is marked even when a different commit in the same
      beam had a failure (partial-failure path).
- [ ] State is per profile: beaming to X does not mark the commits as sent for a
      different profile Y.
- [ ] SHAs that are no longer ahead of the remote are pruned from the local
      config file on the next beam.
- [ ] `go build ./...` passes, `go vet ./...` is clean, and `go test ./...`
      passes (including existing TUI tests).
