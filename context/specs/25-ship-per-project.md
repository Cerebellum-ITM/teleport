# Unit 25: Ship per project — move bin profiles to the local config

## Goal

Fix the design defect in `teleport ship` (unit 14): bin profiles live in the
**global** config keyed only by OS, so one machine can hold exactly one ship
destination per OS across *all* projects. Move the full `BinProfile` (host,
bin_path, remote_name, bin_file) into the **per-project local config**, still
keyed by OS, and auto-migrate existing global entries on first use. Each repo
becomes autonomous: different projects can ship different binaries to
different servers without interfering.

## Bug being fixed

Observed with the current global model (`~/.config/teleport/config.toml`
containing `bin_profiles.linux` with `remote_name = "echo_cli"` and
`bin_file = "bin/echo_cli_linux_amd64"`, configured for the echo_cli repo):

1. **`bin_file` shadowing** — `resolveShipContext` (`cmd/ship.go`) finds the
   global `bin_file` first and returns it for *any* cwd, so a project's local
   `bin_dir` picker is unreachable: running `teleport ship` in another repo
   stats `bin/echo_cli_linux_amd64` relative to that cwd and fails (or,
   worse, ships an unrelated file if the path happens to exist).
2. **`remote_name` clobbering** — passing an explicit binary
   (`teleport ship ./bin/foo`) from any repo still applies the global
   `remote_name`, silently renaming the upload to `echo_cli` and overwriting
   another project's remote binary (`mv -f` is silent by design).
3. Only `bin_dir` is per-project today; host/bin_path/remote_name/bin_file are
   machine-wide, which contradicts the product model (each folder is a
   project and may target a different server).

## Design

### Config model

`LocalConfig` (per-project file under `~/.config/teleport/projects/<hash>.toml`)
gains the bin profile map; the shape of `BinProfile` is unchanged:

```toml
default_profile = "myproj"
bin_dir = "./bin"

[bin_profiles.linux]
host = "vps"
bin_path = "/home/user/.local/bin"
remote_name = "mycli"
bin_file = "bin/mycli_linux_amd64"
```

- Valid keys remain `linux | macos | windows`; validation moves to
  `LoadLocal` (same error message as today).
- `GlobalConfig.BinProfiles` stays in the struct **only so migration can read
  and clear it**; it is deprecated and no command resolves ship destinations
  from it anymore.
- `bin_file` stays relative to the project root (cwd), which is now safe
  because the value only exists inside that project's local config.
- `bin_dir` keeps its current role: fallback picker source when no
  `bin_file` matches.

### Resolution order (after this unit)

1. Explicit positional arg → wins, profile looked up by OS in **local**
   `bin_profiles`.
2. Local `bin_profiles[*].bin_file` (narrowed by `--os` when given).
3. Local `bin_dir` → single file or picker.
4. Error: `no bin profile configured for <os> in this project — run "teleport init"`.

The global config is never consulted for ship resolution.

### Auto-migration

Trigger: `teleport ship` or `teleport init` runs in a project whose local
config has **no** `bin_profiles`, while the global config **has** at least one.

1. Show the global entries and ask (huh confirm):
   `Adopt the global bin profile(s) into this project?` — the prompt makes
   clear the global section is deprecated.
2. **Yes** → copy all entries into the local config verbatim, save it, then a
   second confirm: `Remove [bin_profiles] from the global config?`
   (default **No**, so another repo that also relied on them can still adopt).
   On yes, delete the section and `SaveGlobal`.
3. **No** → continue without them (ship falls through to `bin_dir`/error);
   re-offer on the next trigger.
4. Non-interactive contexts (headless mode / no TTY): never prompt — ignore
   the global section and log a one-line warning
   (`global [bin_profiles] is deprecated — run "teleport ship" interactively to migrate`).

While a global `[bin_profiles]` section still exists, `teleport profiles`
labels it `(deprecated — global)`.

### Out of scope

- Multiple bin profiles per OS within one project.
- Any change to the ship transfer flow itself (upload/chmod/mv/sudo).
- Migrating `bin_dir` (already local).

## Implementation

### `internal/config/config.go`

- Add `BinProfiles map[string]BinProfile \`toml:"bin_profiles,omitempty"\`` to
  `LocalConfig`.
- Move the OS-key validation loop from `LoadGlobal` to `LoadLocal` (keep it in
  `LoadGlobal` too — the field is still decodable there and garbage keys
  should still fail loudly).
- Add `(*LocalConfig) SetBinProfile(os string, p BinProfile)` and
  `RemoveBinProfile(os string)` mirroring the global helpers. Keep the global
  helpers (migration + `RemoveBinProfile` used to clear the section).

### `cmd/ship.go`

- `runShip` / `resolveShipContext`: load the **local** config first; run the
  migration flow (new helper `maybeMigrateBinProfiles(localCfg) error` —
  loads global, prompts, saves) before resolving anything.
- Replace both `globalCfg.BinProfiles` lookups with `localCfg.BinProfiles`.
- Update the missing-profile error to the per-project wording (Design §
  resolution order, step 4).
- `resolveShipContext` should load the local config once and reuse it for the
  `bin_file` scan and the `bin_dir` fallback (today it loads global then
  local; afterwards only local is needed).

### `cmd/init.go`

- `configureBinProfile` receives and writes the **local** config
  (`localCfg.SetBinProfile`) instead of the global one; `SaveLocal` at the
  end of the flow.
- Run the same migration offer before configuring, so `init` in an old setup
  starts from the adopted values.
- The `needBinDir` check (`p.BinFile == ""`) now reads the local map.

### `cmd/profiles.go`

- The `Bin profiles:` section lists the **local** project's `bin_profiles`.
- If the deprecated global section is non-empty, append its entries tagged
  `(deprecated — global)` so the user can see what's pending migration.

### `cmd/config.go`

- `teleport config show`: render local bin profiles alongside `bin-dir`
  (host:path and optional remote_name/bin_file per OS); keep `<unset>`
  placeholder when empty.
- No new settable keys — bin profiles are edited via `teleport init` (same as
  today).

### `cmd/help.go` / `README.md` / `CHANGELOG.md`

- Update the ship/init descriptions to say bin profiles are per-project.
- CHANGELOG entry under `[BREAKING]`-style note: global `[bin_profiles]` is
  deprecated, auto-migration on first interactive run.

## Dependencies

None — no new packages.

## Verify when done

- [ ] `go build ./... && go vet ./...` clean.
- [ ] Repo A and repo B can each hold `bin_profiles.linux` pointing at
  different hosts/names; `teleport ship` in each repo uses its own.
- [ ] With the old global `bin_profiles.linux` present and a fresh project:
  `teleport ship` offers adoption; accepting writes the local section and
  ship resolves from it; declining leaves ship on the `bin_dir` fallback.
- [ ] Second confirm removes the global section only when accepted;
  declining keeps it and `teleport profiles` tags it `(deprecated — global)`.
- [ ] Headless/no-TTY run with only a global section: no prompt, warning
  logged, global values not used.
- [ ] `teleport ship ./bin/foo` in a repo with no local `remote_name` ships
  as `foo` — the old global `remote_name` no longer renames it.
- [ ] `teleport ship` in a repo without `bin_file` never stats another
  project's `bin_file` path (shadowing bug gone).
- [ ] Invalid OS key in a local `[bin_profiles.bsd]` fails `LoadLocal` with
  `unknown bin profile OS "bsd" (expected linux|macos|windows)`.
- [ ] `teleport init` configuring a bin profile writes it to the project's
  local TOML; the global file is untouched.
- [ ] `teleport profiles` and `teleport config show` display the local bin
  profiles.
