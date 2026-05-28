# Unit 14: Ship — deploy a CLI binary to a remote `bin/`

## Goal

Add `teleport ship <bin>` to automate the repetitive flow of copying a locally
built CLI binary to a remote host's executable directory:

1. SFTP upload to a temp path
2. `chmod +x`
3. Move into the host's `bin/` directory (using `sudo` automatically if the
   target path is not writable by the SSH user)

Selection of the destination is driven by the binary's **target OS** (detected
from its magic bytes), with at most three bin profiles per machine — one for
`linux`, one for `macos`, one for `windows`.

## Design

### Subcommand

```
teleport ship <path-to-bin> [--os linux|macos|windows] [--to <remote-dir>] [--name <basename>]
```

- `<path-to-bin>` — local binary file (positional, required).
- `--os` — override the auto-detected target OS (selects the bin profile).
- `--to` — override the bin profile's `bin_path` for this run only.
- `--name` — rename the binary on the remote (default: `filepath.Base(<path>)`).

Examples:

```
teleport ship ./teleport
teleport ship ./dist/mycli-linux --os linux
teleport ship ./mycli --to /opt/local/bin
teleport ship ./mycli --name mycli-dev
```

### Bin profile model

A bin profile is **keyed by target OS**. The global config gains a second map:

```toml
[bin_profiles.linux]
host = "vps"
bin_path = "/usr/local/bin"

[bin_profiles.macos]
host = "mac-mini"
bin_path = "/usr/local/bin"
```

Constraints:

- Valid keys: `linux`, `macos`, `windows`. Any other key is a config error.
- At most one entry per OS (enforced by being a `map`).
- Independent from `[profiles.*]` (the existing sync profiles). A project may
  have both, only sync, or only bin profiles. They are matched by OS, not by
  the sync profile selected in `.teleport.toml`.

Default `bin_path` suggested in the init flow:

| OS      | Default suggestion       |
| ------- | ------------------------ |
| linux   | `/usr/local/bin`         |
| macos   | `/usr/local/bin`         |
| windows | (no default — user picks via dir browser) |

The user can accept the suggestion or pick another directory using the
existing remote dir browser TUI.

### Target OS detection

`teleport ship` reads the first 8 bytes of the local binary and classifies:

| Magic bytes (hex)         | OS       |
| ------------------------- | -------- |
| `7F 45 4C 46`             | linux    (ELF) |
| `FE ED FA CE` / `FE ED FA CF` / `CE FA ED FE` / `CF FA ED FE` / `CA FE BA BE` | macos (Mach-O, incl. fat) |
| `4D 5A`                   | windows  (PE / `MZ`) |
| anything else             | error: not a recognised executable binary |

When `--os` is supplied, detection is skipped and the override is honored.

### Profile resolution

1. Determine target OS (detection or `--os`).
2. Look up `bin_profiles[<os>]` in the global config.
3. If missing → error:
   `no bin profile configured for <os> — run "teleport init" and add one`.
4. Resolve `bin_path`: `--to` flag wins; otherwise use `profile.bin_path`.

### Ship flow

Given resolved `(host, bin_path, localPath, remoteName)`:

1. **Pre-flight**: `os.Stat(localPath)` → regular file; magic-byte check.
2. **Connect** via the existing `connectToHost` helper (key/agent → password
   fallback is reused as-is).
3. **Upload** the binary to `/tmp/teleport-ship-<rand>/<remoteName>` via
   `client.UploadFile` (parents auto-created).
4. **chmod**: `chmod +x <tmp>` via `client.RunCommand`.
5. **Writability probe**: `[ -w <bin_path> ] && echo y || echo n`.
   - `y` → move with plain `mv -f <tmp> <bin_path>/<remoteName>`.
   - `n` → move with `sudo -n mv -f …`. If `sudo -n` fails because a password
     is required (exit status non-zero with `a password is required` /
     `sudo: a terminal is required` on stderr), prompt the user with the
     existing masked `huh` password input and re-run as
     `echo <pw> | sudo -S mv -f …`. Password is held only in memory for the
     duration of this command.
6. **Cleanup**: `rmdir <tmp-dir>` (best-effort, warn on failure).
7. **Output** — single-line success:
   ```
    shipped teleport → vps:/usr/local/bin/teleport (linux, sudo)
   ```
   Uses the package's existing OK style. The trailing `(linux, sudo)` notes
   the detected OS and whether sudo was used.

Overwrite is silent (`mv -f`), per decision.

### `teleport init` — choose sync, bin, or both

`teleport init` is extended so the user can configure a sync profile, one or
more bin profiles, or both in a single session.

New first step (huh `MultiSelect`):

```
What do you want to configure?
  [x] Sync profile (teleport sync / beam / pull / status / clean)
  [ ] Bin profile — Linux  (teleport ship)
  [ ] Bin profile — macOS  (teleport ship)
  [ ] Bin profile — Windows (teleport ship)
```

At least one option must be selected. The existing sync flow runs only if
"Sync profile" is checked. For each selected bin OS, the flow asks:

1. Reuse the host picker (single shared invocation if multiple bin OSes share
   a host is **not** an optimization in this unit — pick host per OS, simple).
2. Show the dir browser, starting at the OS default
   (`/usr/local/bin` for linux/macos, `/` for windows). The header changes to
   `Select bin/ directory for <os>`.
3. Persist as `bin_profiles[<os>]` via the new `SetBinProfile` helper.

If only bin profiles are selected (no sync), `.teleport.toml` is **not**
written — sync's `default_profile` only matters for sync-family commands.

### Sudo handling — details

- The probe runs as a single shell command; exit-status drives the branch.
  No reliance on stderr parsing for the writable-or-not decision.
- `sudo -n` is attempted first to catch the passwordless case silently.
- On a password prompt, the user is shown:
  ```
  sudo password for <user>@<host>:
  ```
  using the same `huh.NewInput().Password()` pattern as
  `cmd/clean.go`'s SSH password prompt.
- If the user cancels the password prompt, the command exits with the binary
  still parked under `/tmp/teleport-ship-<rand>/`; the message points the
  user there so they can finish manually if they want:
  ```
  aborted — binary left at vps:/tmp/teleport-ship-<rand>/teleport
  ```

### Out of scope

- Versioned filenames / symlinks (`mycli-1.2.3 → mycli`).
- Backup of the existing remote binary (overwrite is silent by decision).
- Multiple binaries per invocation (one `<bin>` per command for v1).
- Shipping to a remote that is not on the SSH host's `PATH`
  (warning only, not enforced).
- Cross-OS rejection: shipping a Mach-O to a Linux bin profile via `--os`
  override is allowed; we trust the user.

## Implementation

### `internal/config/config.go`

Add the bin profile type and accessors:

```go
type BinProfile struct {
    Host    string `toml:"host"`
    BinPath string `toml:"bin_path"`
}

type GlobalConfig struct {
    Profiles    map[string]Profile    `toml:"profiles"`
    BinProfiles map[string]BinProfile `toml:"bin_profiles,omitempty"`
}

func (g *GlobalConfig) SetBinProfile(os string, p BinProfile) {
    if g.BinProfiles == nil {
        g.BinProfiles = make(map[string]BinProfile)
    }
    g.BinProfiles[os] = p
}

func (g *GlobalConfig) RemoveBinProfile(os string) {
    delete(g.BinProfiles, os)
}
```

Validate on load: any key in `BinProfiles` not in
`{"linux","macos","windows"}` is a decoding error
(`unknown bin profile OS %q (expected linux|macos|windows)`).

### `internal/ssh/client.go`

Reuse what exists; add only:

```go
// RemoteWritable returns true if the SSH user can write to dir on the remote.
func (c *Client) RemoteWritable(dir string) (bool, error)
```

Implementation: `c.RunCommand("[ -w " + ShellQuote(dir) + " ] && echo y || echo n")`.
Trim the output, compare to `"y"`.

No new transport types; sudo is handled in `cmd/ship.go` via `RunCommand` +
existing password prompt helper.

### `internal/bindetect/bindetect.go` — new package

Tiny single-purpose package — magic-byte sniffer:

```go
package bindetect

type OS string

const (
    Linux   OS = "linux"
    MacOS   OS = "macos"
    Windows OS = "windows"
)

func Detect(path string) (OS, error)
```

Reads the first 8 bytes of `path`, matches against the table in the design
section, returns an error if no match. Lives in `internal/` because it has
zero ties to TUI/SSH/git/config — pure file inspection.

### `internal/tui/dirpicker.go`

Add an optional starting-path argument override (already supported by
`RunDirPicker(client, startPath string)`). The init flow will pass
`/usr/local/bin` (linux/macos) or `/` (windows). No code changes — only
documenting the call sites.

The header text (`Select remote directory`) becomes parameterized via a new
`RunDirPickerWith(client, startPath, header string)` helper; the existing
`RunDirPicker` keeps its current header by calling the new one. This keeps
the init flow's three (or four) consecutive dir pickers visually distinct.

### `cmd/ship.go` — new file

Thin command, no business logic beyond orchestration:

```go
var (
    shipOS   string
    shipTo   string
    shipName string
)

var shipCmd = &cobra.Command{
    Use:   "ship <bin>",
    Short: " deploy a local binary to its OS-matching bin profile",
    Args:  cobra.ExactArgs(1),
    RunE:  runShip,
}

func init() {
    shipCmd.Flags().StringVar(&shipOS, "os", "", "override target OS (linux|macos|windows)")
    shipCmd.Flags().StringVar(&shipTo, "to", "", "override remote bin directory for this run")
    shipCmd.Flags().StringVar(&shipName, "name", "", "rename binary on the remote")
    rootCmd.AddCommand(shipCmd)
}

func runShip(_ *cobra.Command, args []string) error {
    localPath := args[0]

    // 1. Pre-flight
    info, err := os.Stat(localPath)
    if err != nil {
        return fmt.Errorf("stat %s: %w", localPath, err)
    }
    if !info.Mode().IsRegular() {
        return fmt.Errorf("%s is not a regular file", localPath)
    }

    // 2. Determine target OS
    targetOS := bindetect.OS(shipOS)
    if targetOS == "" {
        targetOS, err = bindetect.Detect(localPath)
        if err != nil {
            return fmt.Errorf("detect binary type: %w", err)
        }
    }

    // 3. Resolve bin profile
    globalCfg, err := config.LoadGlobal()
    if err != nil {
        return fmt.Errorf("load global config: %w", err)
    }
    profile, ok := globalCfg.BinProfiles[string(targetOS)]
    if !ok {
        return fmt.Errorf("no bin profile configured for %s — run \"teleport init\" and add one", targetOS)
    }
    binDir := profile.BinPath
    if shipTo != "" {
        binDir = shipTo
    }
    remoteName := shipName
    if remoteName == "" {
        remoteName = filepath.Base(localPath)
    }

    // 4. Connect (reuse existing helper)
    client, err := connectToHost(sshpkg.Host{Name: profile.Host})
    if err != nil {
        return err
    }
    defer client.Close()

    // 5. Upload to tmp
    tmpDir := fmt.Sprintf("/tmp/teleport-ship-%d", rand.Int63())
    tmpPath := tmpDir + "/" + remoteName
    if err := client.UploadFile(localPath, tmpPath); err != nil {
        return fmt.Errorf("upload: %w", err)
    }

    // 6. chmod +x
    if _, err := client.RunCommand("chmod +x " + sshpkg.ShellQuote(tmpPath)); err != nil {
        return fmt.Errorf("chmod: %w", err)
    }

    // 7. Move into bin_path (sudo if needed)
    finalPath := binDir + "/" + remoteName
    writable, err := client.RemoteWritable(binDir)
    if err != nil {
        return fmt.Errorf("probe %s: %w", binDir, err)
    }
    usedSudo := false
    moveCmd := fmt.Sprintf("mv -f %s %s",
        sshpkg.ShellQuote(tmpPath), sshpkg.ShellQuote(finalPath))
    if writable {
        if _, err := client.RunCommand(moveCmd); err != nil {
            return fmt.Errorf("mv: %w", err)
        }
    } else {
        usedSudo = true
        if err := runWithSudo(client, profile.Host, moveCmd, tmpPath); err != nil {
            return err
        }
    }

    // 8. Cleanup tmp dir (best-effort)
    _, _ = client.RunCommand("rmdir " + sshpkg.ShellQuote(tmpDir))

    // 9. Output
    suffix := string(targetOS)
    if usedSudo {
        suffix += ", sudo"
    }
    fmt.Printf("%s shipped %s → %s:%s (%s)\n",
        okStyle.Render(""), remoteName, profile.Host, finalPath, suffix)
    return nil
}
```

`runWithSudo` is a helper local to `cmd/ship.go`:

```go
// runWithSudo tries `sudo -n` first; if a password is required, prompts
// the user with a masked input and pipes it to `sudo -S`. On user
// cancellation, returns an error noting where the tmp file was left.
func runWithSudo(c *sshpkg.Client, host, cmd, tmpPath string) error
```

### `cmd/init.go`

Insert the multi-select step at the top of `runInit`. Keep the existing sync
flow inside an `if syncSelected` block. After the sync section, loop over
selected bin OSes and run a per-OS sub-flow:

```go
for _, osName := range selectedBinOSes {
    if err := configureBinProfile(osName, globalCfg); err != nil {
        return err
    }
}
```

`configureBinProfile` lives in `cmd/init.go`:

1. Suggest the OS default `bin_path` (table above).
2. Reuse the host picker (`RunHostPicker`).
3. Open `RunDirPickerWith` starting at the default with header
   `Select bin/ directory for <os>`.
4. `globalCfg.SetBinProfile(osName, …)`.

`.teleport.toml` is written only if sync was selected (existing behaviour
when no bin section is present).

### `cmd/help.go`

Add `ship <bin>` to the command table with a one-line description.

### `cmd/profiles.go` (optional within this unit)

`teleport profiles` lists `[profiles]`. Extend the printed output to also
list bin profiles in a separate section:

```
Sync profiles:
  vps           dev.example.com:/srv/app  *
  staging       stg.example.com:/srv/app

Bin profiles:
  linux         vps:/usr/local/bin
  macos         mac-mini:/usr/local/bin
```

If the bin map is empty, omit the section.

## Dependencies

No new third-party packages.

## Verify when done

- [ ] `go build -o teleport . && go vet ./...` clean.
- [ ] `teleport init`: multi-select shows sync + 3 bin OS options; selecting
  only "Bin profile — Linux" runs host picker → dir picker (starts at
  `/usr/local/bin`) → saves `[bin_profiles.linux]` in global TOML; no
  `.teleport.toml` is written.
- [ ] `teleport init`: selecting "Sync" + "Bin (Linux)" runs both flows and
  writes both sections.
- [ ] `teleport ship ./teleport` on a Linux-built binary: detects ELF →
  uploads → chmods → moves to `/usr/local/bin/teleport`. If the SSH user
  lacks write perms, prompts for sudo password and succeeds.
- [ ] `teleport ship ./mycli --os macos`: ignores magic bytes, uses the
  macos bin profile.
- [ ] `teleport ship ./mycli --to /opt/bin`: overrides bin path for this
  run only; global TOML is not modified.
- [ ] `teleport ship ./mycli --name mycli-dev`: lands on the remote as
  `mycli-dev`.
- [ ] `teleport ship ./README.md`: errors with
  `detect binary type: not a recognised executable binary (no ELF/Mach-O/PE magic)`.
- [ ] `teleport ship ./teleport` with no `bin_profiles.linux` configured:
  errors with `no bin profile configured for linux — run "teleport init" …`.
- [ ] Sudo password prompt cancelled (`ctrl+c`): exits with
  `aborted — binary left at <host>:/tmp/teleport-ship-…/<name>`; nothing
  is moved into the bin dir.
- [ ] Overwrite of an existing remote binary is silent and successful
  (no prompt).
- [ ] Manually crafted invalid `bin_profiles.bsd` entry in TOML: load fails
  with `unknown bin profile OS "bsd"`.
- [ ] `teleport profiles` shows both sections when both exist; only "Sync
  profiles" when bin map is empty.
- [ ] `teleport ship --help` lists `--os`, `--to`, `--name`.
