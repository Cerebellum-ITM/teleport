# Unit 26: Shell shortcut flag (`--sh`)

## Goal

Add a root-level shortcut so the user can open the remote interactive shell
without typing the full `shell` subcommand — `teleport --sh [profile]` dispatches
to the existing `teleport shell` (Unit 16). The remote shell stays `zsh`; this
unit is **only** the shortcut, not a shell-selection or remote-command feature.

## Design decision — why `--sh`, not literal `-sh`

The user asked for "`-sh`". In cobra/pflag a **shorthand is exactly one letter**,
so `-sh` cannot be registered as a single flag. Worse, `-s` is already the
`--sync` shorthand, so pflag would parse the token `-sh` as the cluster
`-s` + `-h` → it would trigger **sync + help**, never a shell. Therefore the
shortcut is the long boolean flag `--sh` (no shorthand), which slots cleanly into
the existing root-dispatch pattern next to `--sync`/`--init`/`--profiles`/`--beam`.

`teleport shell [profile]` (Unit 16) is unchanged and remains the canonical form;
`--sh` is sugar over it.

## Design

### Behavior

```
teleport --sh            # open shell using the wd's default_profile
teleport --sh staging    # open shell using the named profile
```

`--sh` is a boolean root flag. When set and no subcommand is given, `rootCmd.RunE`
calls `runShell(cmd, args)` — the exact same entrypoint as `teleport shell`,
reusing `resolveProfile(args)`, `execSSH`, and the `zsh` remote shell. Any
positional arg is forwarded as the profile name (mirrors how `--beam`/`--sync`
forward `args`).

Follows the established root-shortcut convention: `-s`/`--sync`, `-i`/`--init`,
`-p`/`--profiles`, `-b`/`--beam` are all boolean flags on `rootCmd.Flags()` whose
`RunE` switch dispatches to the matching `runX`. `--sh` adds one more `case`.

### Interaction with headless/JSON

`shell` execs the system `ssh` and is inherently interactive (Unit 16); it does
not participate in `--json`/`--no-input`. `--sh` inherits that unchanged — no new
headless contract. (If `--sh` is combined with `--no-input`, behavior is identical
to `teleport shell --no-input`: it still execs ssh, which will simply fail if no
TTY is present — no teleport-side guard is added, matching Unit 16.)

## Implementation

### `cmd/root.go`

1. Add the state var next to the other root dispatch bools:

   ```go
   var rootShell bool
   ```

2. Register the flag in `init()` (long-only, no shorthand — see design note):

   ```go
   rootCmd.Flags().BoolVar(&rootShell, "sh", false, " open an interactive shell on the remote")
   ```

3. Add the dispatch `case` in `rootCmd.RunE`'s switch, before `default`:

   ```go
   case rootShell:
       return runShell(cmd, args)
   ```

   Order relative to the other cases is irrelevant (the flags are mutually
   exclusive in practice; first-set wins, same as today).

No change to `cmd/shell.go`, `cmd/shell_unix.go`, or `cmd/shell_windows.go` —
`runShell` is reused as-is.

### `cmd/help.go`

- Add a Global-Flags row after the `--beam` row:

  ```go
  {"", "--sh", iconGear, "open an interactive shell on the remote"},
  ```

  (No shorthand column value, matching `--no-input`/`--json`.)
- Add an example near the other shortcut examples:

  ```go
  {"teleport --sh", "open a shell on the remote (shortcut for `teleport shell`)"},
  ```

### Helpers reused (nothing reimplemented)

- `runShell` — `cmd/shell.go` (Unit 16).
- `resolveProfile` — `cmd/clean.go`.

## Dependencies

None (no new third-party packages; pure wiring in `cmd/`).

## Verify when done

- [ ] `go build -o teleport . && go vet ./... && gofmt -l cmd/` clean.
- [ ] `GOOS=windows go build ./...` still compiles (no new platform code).
- [ ] `teleport --sh` opens the same `zsh` session as `teleport shell` sitting in
  `profile.Path` (verify `pwd` / `echo $0`).
- [ ] `teleport --sh <profile>` uses that profile's host and path.
- [ ] `teleport -sh` (single dash) is documented as **not** the trigger — confirm
  it errors or triggers sync+help as pflag dictates, and the help/README steer
  users to `--sh`. (No attempt is made to make the two-letter single-dash form work.)
- [ ] `teleport shell` and `teleport shell <profile>` behave exactly as before.
- [ ] `teleport -h` lists `--sh` under Global Flags and shows the example.
- [ ] `sync`/`beam`/`init`/`profiles` root shortcuts are unaffected (the new
  `case` is additive).
- [ ] Non-existent profile → clear error from `resolveProfile`; no `ssh` in PATH →
  the Unit 16 error path.
