# AI Workflow Rules

## Approach

Build this project incrementally using a spec-driven workflow. The six context
files define what to build, how to build it, and the current state of progress.
Always implement against these specs — do not infer or invent behavior not
described in the context files. When a spec is missing, create it first with
`/spec-driven-dev spec NN nombre` before writing any code.

## Scoping Rules

- Work on one feature unit at a time.
- Prefer small, verifiable increments over large speculative changes.
- Do not combine changes to `internal/ssh`, `internal/tui`, and `cmd/` in the same step unless the spec explicitly covers all three.
- Do not add error handling, fallbacks, or validation for scenarios that can't happen. Trust internal package guarantees.
- Do not refactor code that is not in scope for the current unit.

## When to Split Work

Split an implementation step if it combines:

- A new TUI component and new SSH/SFTP logic in the same step.
- Changes to multiple unrelated `internal/` packages.
- Behavior that is not fully defined in the current spec — stop and clarify first.

If a change cannot be verified end to end in one `go build` + manual run cycle, the scope is too broad — split it.

## Handling Missing Requirements

- Do not invent product behavior not defined in `context/project-overview.md`.
- If a requirement is ambiguous, stop and add it as an open question in `progress-tracker.md` before continuing.
- If a spec section says `_TBD_`, do not assume — ask the user to resolve it.

## Protected Files

Do not modify the following unless explicitly instructed:

- `go.sum` — managed by `go get` / `go mod tidy` only; never edit by hand.
- `go.mod` — add dependencies only via `go get`; do not edit the `require` block manually.
- Any file under the module cache (`$GOPATH/pkg/mod/`).

## Keeping Docs in Sync

Update the relevant context file whenever implementation changes:

- A new package or command added → update `context/architecture.md` (System Boundaries).
- A new dependency added → update `context/architecture.md` (Stack table).
- A coding pattern changes → update `context/code-standards.md`.
- A feature added or removed → update `context/project-overview.md` (Features, Scope).

## Before Moving to the Next Unit

1. `go build -o teleport .` completes with zero errors.
2. The new behavior works end to end in a manual run (or a clear reason why it can't be tested).
3. No invariant defined in `context/architecture.md` was violated.
4. `context/progress-tracker.md` reflects the completed unit.
5. No unrelated files were modified as a side effect.
