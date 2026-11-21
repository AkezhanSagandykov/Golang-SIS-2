# AGENTS.md — instructions for AI coding agents

These rules apply to any AI assistant or coding agent working in this repository
(Claude, Codex, Copilot, Cursor and similar). Humans follow them too.
`CLAUDE.md` only imports this file.

## 1. Purpose

TaskFlow is a small console task tracker written in Go (university course project,
Checkpoints 1–2). Users create tasks, submit them for **description analysis**, and a
fixed **worker pool** processes them in the background. Data lives **only in memory**;
there is no database, network service, login or deployment target.

## 2. Architecture

```
cmd/taskflow/main.go     wiring: flags, MemoryStore + Service + CLI, Shutdown on exit
internal/task/           domain: Task, Status + transition table, Priority, Analyze, errors
internal/storage/        MemoryStore: map + sync.RWMutex, atomic Update(id, change)
internal/worker/         Pool: buffered chan task.ID, N workers, non-blocking Submit, Stop
internal/service/        use cases: Create/Submit/process/Cancel/Reopen/List/Stats/Shutdown
internal/cli/            command parsing and output (user-facing text is Russian)
```

Dependency direction: `cli → service → task`, `cli → worker` (only to recognise
`ErrQueueFull` / `ErrStopped` in messages), `service → worker`, `storage → task`.
`service` knows storage only through the `Repository` interface; `main` wires them.
`storage` and `worker` must never import `service` or `cli`.

Invariants that must not be broken (see `docs/assignment05/acceptance-criteria.md`):

- Status changes go only through the table in `internal/task/status.go`.
- Check-and-change of a task happens inside one `MemoryStore.Update` call (one lock).
- The store returns copies (`Clone()`); slices are never shared between goroutines.
- `Pool.Submit` never blocks; a full queue or a stopped pool returns an error and the task goes back to `todo`.
- Only the pool closes its channel; `Stop` lets workers finish every accepted task.

## 3. Setup

- Go 1.22 or newer (`go.mod`: `go 1.22`). No third-party modules: `go.sum` does not exist.
- Run every command from the repository root (the folder with `go.mod`).
- Use the installed Go (`GOTOOLCHAIN=local`): no command may download a toolchain.
- Run the app: `go run ./cmd/taskflow -demo` (flags: `-workers N` (N ≥ 2), `-queue N`).

## 4. Commands

**Safe — an agent may run these without asking** (read-only or local build/test):

| Purpose | Command |
|---|---|
| Format check (must print nothing) | `gofmt -l .` |
| Static checks | `go vet ./...` |
| Build | `go build ./...` |
| Tests | `go test -count=1 ./...` |
| Race detector (required for concurrency code) | `go test -count=1 -race ./...` |
| Coverage | `go test -count=1 -cover ./...` |
| **All of the above in one gate** (exits non-zero on any failure; CI runs it too) | `./scripts/check.sh` |
| Inspect history | `git status`, `git diff`, `git log` |

**Ask the human first:** `go run ./cmd/taskflow` (interactive), `gofmt -w`, `go get`,
`go mod tidy`, `go install`, any command that uses the network, `git commit`, creating branches.

## 5. Forbidden actions

An agent must never:

1. Add secrets, tokens, passwords, API keys, real names, phone numbers or other real personal data
   to code, tests, fixtures, commit messages or logs. Use obviously fictional sample data.
2. Deploy, publish a release, create tags, push to any remote, or push/force-push to `main`.
3. Run destructive commands: `rm -rf`, `git reset --hard`, `git clean -fdx`, `git push --force`,
   `git checkout -- .`, deleting branches, tests or documentation.
4. Change dependencies without review: adding or upgrading modules in `go.mod`, changing the
   `go`/`toolchain` directive, vendoring, or downloading tools into the repository.
5. Weaken verification: skip or delete tests (`t.Skip`), loosen assertions, remove `-race`,
   or edit files in `docs/assignment05/evidence/` by hand (they hold real command output).
6. Change the status transition table, the locking rules or the pool shutdown order unless
   the task explicitly asks for it and a test proves the new behaviour.

## 6. Coding conventions

- `gofmt` formatting; small focused functions; no new packages without a reason.
- Errors: wrap with `fmt.Errorf("...: %w", err)`; compare with `errors.Is` / `errors.As`;
  use the sentinel and typed errors in `internal/task/errors.go`.
- Library code does not panic on bad input and does not call `os.Exit`.
- Never return internal slices or pointers to stored tasks; return `Clone()`.
- Tests: table-driven where it helps; no `time.Sleep` for synchronisation (use channels);
  every concurrency test must pass under `-race`.
- User-facing CLI messages stay in Russian; identifiers and comments in English.

## 7. Test expectations

- Every behaviour change or bug fix comes with a test that fails before the change.
- Before proposing a diff, run `./scripts/check.sh` (gofmt, vet, build, race tests, coverage).
  It must end with `OK`; report its output. CI (`.github/workflows/checks.yml`) repeats it on every push.
- Coverage of `internal/...` packages must not go down.

## 8. Review gates

1. One focused change per branch; never mix refactoring with behaviour changes.
2. The agent delivers: a change summary, the diff, the exact commands it ran with
   pass/fail totals, and known risks or tests it could not run.
3. A human reads the whole diff and re-runs the checks before anything is committed or merged.
4. Nothing is merged, tagged or released without explicit human approval.
