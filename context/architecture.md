# Architecture Context

## Stack

| Layer        | Technology                            | Role                                              |
| ------------ | ------------------------------------- | ------------------------------------------------- |
| Language     | Go 1.25                               | Single binary, cross-platform CLI                 |
| CLI framework | cobra v1.10                          | Command routing, flag parsing, help generation    |
| TUI framework | charm.land/bubbletea v2.0.6          | Interactive terminal UI (host picker, dir browser, file picker) |
| TUI forms    | charm.land/huh v2.0.0                 | Profile name input form                           |
| TUI components | charm.land/bubbles v2.1.0           | Filterable list component for host picker         |
| TUI styling  | charm.land/lipgloss v2.0.3            | Color tokens and text styling in terminal         |
| Logging      | charmbracelet/log v1.0.0              | Structured terminal log output                    |
| SSH          | golang.org/x/crypto/ssh               | SSH connection, auth (agent + key files)          |
| SFTP         | github.com/pkg/sftp v1.13.10          | Remote file listing and upload                    |
| Config       | github.com/BurntSushi/toml v1.6.0     | Read/write TOML config files                      |

## System Boundaries

- `cmd/` — cobra command definitions; orchestrates packages from `internal/`; owns the user-facing flow
- `internal/config/` — reads and writes global (`~/.config/teleport/config.toml`) and local (`.teleport.toml`) config; no I/O except config files
- `internal/ssh/` — parses `~/.ssh/config`, establishes SSH+SFTP connections, exposes `UploadFile` and `ListDirs`; no config or TUI logic
- `internal/git/` — runs `git ls-files` and `git ls-files --others`; returns plain string slices; no SSH or config logic
- `internal/tui/` — bubbletea models for host picker, dir browser, file picker; pure TUI; no business logic

## Storage Model

- **Global config** (`~/.config/teleport/config.toml`): named profiles with SSH host and remote path. Shared across all projects on the machine.
- **Local config** (`.teleport.toml` in project CWD): one field — `default_profile`. Per-project, not committed to git.
- **No database, no remote state**: all state is in flat TOML files on the local filesystem.

## Auth and Access Model

- Authentication is entirely delegated to the OS SSH stack. `teleport` does not manage credentials.
- Auth precedence: (1) `SSH_AUTH_SOCK` agent, (2) `~/.ssh/id_ed25519`, (3) `~/.ssh/id_rsa`, (4) `~/.ssh/id_ecdsa`.
- Host key verification uses `~/.ssh/known_hosts` when present; falls back to `InsecureIgnoreHostKey` otherwise.
- No user accounts, no tokens, no teleport-specific auth layer.

## Deploy Models: `beam` vs `mirror` (Unit 20)

teleport has two distinct ways to push local work to the remote:

| | `beam` (file-level) | `mirror` (git-faithful) |
| --- | --- | --- |
| Transfers | file *contents* via SFTP | git *objects* via `git bundle` |
| Remote git history | untouched (no commits created) | **advances** to the same SHAs |
| Same hash + message | n/a | **yes**, by construction |
| Remote working tree | left *dirty* (hence `clean`) | left *clean* at the new HEAD |
| Non-contiguous subset | supported (cherry-pick of files) | not supported (gaps break the hash) |

`mirror` bundles the contiguous range `remoteHEAD..chosen` (or full history for
an unborn/diverged remote under `--force`), SFTPs it into the remote `.git/`,
then `git fetch <bundle> refs/teleport/mirror` + `git merge --ff-only`
(or `git reset --hard` under `--force`). It advances **whatever branch the
remote has checked out** — it never runs `git checkout` itself; the operator
selects the destination branch once by checking it out. The commit hash never
depends on the branch name, only ancestry decides fast-forward eligibility.

**Invariant: `mirror` never rewrites a commit.** It only fast-forwards the
remote; the sole non-fast-forward path is an explicit `--force` (`reset --hard`)
behind a confirmation (auto-confirmed only by `-y`/`--no-input`). It keeps no
per-profile state — the remote HEAD is re-read each run as the source of truth.

## `push` — git-agnostic transfer (Unit 28)

`push` is the only transfer command that does **not** consult git: it uploads the
paths it is given, so gitignored build output can reach the remote (`sync -u`
enumerates with `--exclude-standard` and skips ignored paths silently; `beam`/
`mirror` carry only what git has; `ship` accepts executables only). It is the
explicit escape hatch — `sync` is not taught to include ignored files, and
`ship`'s magic-byte validation is not relaxed.

- Destination resolution is a pure function (`cmd.resolvePushDest`) over the
  profile path; the local reference point is the **current working directory**,
  never the git root (push must not call git at all).
- **Invariant: `push` never writes outside the profile's resolved `path`.** `--to`
  is relative-only (absolute values rejected) and every destination is
  `path.Clean`ed and containment-checked against the profile path, so no `..`
  escape reaches the remote. Remote paths use `path`, local paths `filepath`.
- **Invariant: collection completes before the first byte is uploaded.** A missing
  path, broken symlink, symlink cycle or unsupported file type aborts with an
  untouched remote. Symlinks are followed; nothing is skipped silently.
- No deletion in v1 (no rsync-style `--delete`): clean replacement is
  push-to-new-directory plus a swap in a `--then` action. `push` does not touch
  `last_sync`, which describes git parity.

## Headless Mode (Unit 19)

- A single guard, `cmd.interactive()`, decides whether any TUI (prompt, picker, viewer, confirmation) may open: it returns true only when stdin is a real TTY **and** the persistent `--no-input` flag was not set. Every TUI call site consults it; no per-`Opts` boolean is threaded through.
- **Invariant: no TTY (redirected stdin, CI, or `--no-input`) ⇒ never open a TUI.** Headless either resolves the selection/confirmation from flags or fails closed with `ErrNeedsTTY` (exit `2`), naming the missing flag in the error. It never blocks waiting for input.
- The persistent `--json` flag makes each command print one JSON result object to `stdout` and route all decoration/progress to `stderr`. `useTUI()` (`interactive() && !jsonOut`) gates the rich progress views; the plain stderr uploaders in `internal/tui` (`RunSyncPlain`/`RunBeamSendPlain`) are used otherwise so `stdout` stays a clean, parseable object.
- **Exit-code contract:** `0` = success / in sync; `1` = execution error, or drift detected by `status`; `2` = a selection or confirmation could not be resolved from flags without a TTY (`ErrNeedsTTY`). `cmd.Execute` maps errors to this contract and reports them (JSON on stderr under `--json`).

## Actions (Unit 23)

An **action** is a named, ordered sequence of remote commands attached to a
profile in the global config (`[profiles.<name>.actions.<action>]`): `Run []string`,
optional `Cwd *string` (nil = profile path, `""` = no `cd`, else that directory),
optional `Timeout` (Go duration, per step, default 10m), and `Confirm bool`.
Actions run over the profile's SSH host and are **not tied to the profile path** —
each declares its own working directory. Invoke them standalone (`teleport run
<action>...`) or chain them after a successful transfer (`sync`/`beam`/`mirror`
`--then <action>`), reusing the transfer's open connection.

- **Streaming.** `ssh.Client.RunCommandStream(ctx, cmd, onLine)` runs one step on
  a fresh session, delivering each stdout/stderr line to `onLine` as it arrives
  (two `bufio.Scanner` goroutines, serialized; **no PTY**, so output stays clean
  and parseable). It returns the remote exit code; a non-zero exit is reported via
  the code, not an error (err is non-nil only for session/transport/timeout). The
  presentation lives in `internal/tui.ActionSink`, a streaming printer (not a
  bubbletea program): colored to stdout when `useTUI()`, else uncolored `[name]`-
  prefixed lines to stderr — so `--json` keeps stdout clean and headless callers
  still see the logs. The SSH I/O stays in `cmd/` (invariant #3).
- **Invariant: an action's steps run in order; the first non-zero step aborts the
  action** (and, under `--then`, the chain), and the command exits `1`. A transfer
  that succeeds but whose `--then` action fails nests the per-action result in the
  command's JSON so callers distinguish "shipped but didn't deploy".
- **Invariant: `--then` action names are validated against the profile before the
  transfer starts** (a typo, or a headless `confirm`-action without `-y`, fails
  before anything is uploaded). `Confirm` follows the `clean` rule: interactive
  prompts; headless auto-confirms only with `-y`/`--no-input`, else `ErrNeedsTTY`.

## Invariants

1. `internal/` packages must never import `cmd/` — dependency flow is strictly `cmd → internal`.
2. SSH and SFTP connections are always created in `cmd/` layer (or delegated to `internal/ssh`), never inside TUI models.
3. `internal/tui` models must not perform I/O directly — they receive data via messages and emit selections; callers do the I/O.
4. Config writes happen only after all interactive steps complete successfully — a cancelled TUI flow must not write partial config.
5. `go build` must produce a zero-dependency static binary (no CGO).
6. SFTP uploads must never report success on a truncated transfer: every upload closes the remote handle explicitly and propagates its error, and verifies the remote size equals the local size before returning `nil`. The SFTP write packet stays within OpenSSH's `SFTP_MAX_MSG_LENGTH` (32 KB via `MaxPacket(32768)`); throughput comes from concurrent/pipelined writes, never from oversized packets.
