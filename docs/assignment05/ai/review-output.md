# Output of the read-only review agent (Task 4), verbatim

Returned 2026-11-21 by the Claude "Plan" sub-agent (model Claude Opus 5.5). Line numbers refer to tag
`a05-reviewed` (logged as `f07c483`, now `275c985`). My classification of each finding is in the report, section 4,
and the probes that verify it are in `../evidence/05b_review_verification_at_a05-reviewed.txt`.

---

# Risk review of TaskFlow at tag `a05-reviewed` (commit f07c483)

I found 12 candidate risks. The most serious one was introduced by the reviewed commit itself: any input line whose first word is `exit` or `quit` now ends the session, and ending the session loses every task. All six AGENTS.md gates pass at this version.

**How I reviewed it.** I read every file at the tag with `git show a05-reviewed:<path>`. The working tree matches the tag for all Go code, so I ran the gates there:
- `git diff --stat a05-reviewed HEAD` shows only `docs/assignment05/evidence/05_pattern_after.txt | 82 +++`.
- `git status --porcelain` printed nothing (clean tree).
- `go version` prints `go1.24.7 linux/amd64`.

The AGENTS.md block in my prompt arrived as the literal `{AGENTS}`. I read the file at the tag; the corrected text you sent later is the same.

**Gates I ran:**

| Command | Result |
|---|---|
| `gofmt -l .` | no output, exit 0 |
| `go vet ./...` | exit 0 |
| `go build ./...` | exit 0 |
| `go test -count=1 ./...` | all 5 packages ok, exit 0 |
| `go test -count=1 -race ./...` | all 5 packages ok, exit 0 |
| `go test -count=1 -cover ./...` | cli 94.7% (baseline file says 93.9%), service 95.9%, storage 100.0%, task 98.2%, worker 100.0% |

**Other checks:**
- Every test named in `acceptance-criteria.md` exists (each name found once with `git grep`).
- `git grep` found no `time.Sleep`, no `recover()`, no `signal.Notify` and no control-character checks in Go code.
- The tree has no CI, no Makefile, no lint config and no probe files.

**Not executed:** `go run` (needs your approval per AGENTS.md), govulncheck and staticcheck (would need the network), and any new probe test (would need new files). Where I rely on a baseline probe in `docs/assignment05/evidence/` (captured at cfa20b2), I say so. In those cases the code involved has not changed since: `git diff --stat cfa20b2 a05-reviewed -- internal/service internal/worker internal/storage internal/task cmd` printed nothing.

## Summary

| ID | Title | Category | Confidence | L | I | L×I |
|---|---|---|---|---|---|---|
| R1 | A line over 64 KiB ends the session and loses every task | Correctness | Confirmed | 2 | 3 | 6 |
| R2 | Any line starting with `exit`/`quit` (any case, any arguments) ends the session | Correctness | Confirmed | 1 | 3 | 3 |
| R3 | A panic in a processor crashes the whole process; tasks in queued/in_progress have no way out | Correctness | Confirmed (crash) / Speculative (stuck task) | 1 | 3 | 3 |
| R4 | Deadline "end of day" is wrong on daylight-saving days | Correctness | Confirmed (code), not executed | 1 | 1 | 1 |
| R5 | The documented package map is already wrong (`cli → worker`) and no test checks package boundaries | Maintainability | Confirmed | 2 | 2 | 4 |
| R6 | The new command-table test checks the table against itself; some AC5 messages are never asserted | Maintainability | Confirmed | 2 | 2 | 4 |
| R7 | User text is printed raw (no control-character filter, no size limit) | Security | Plausible | 1 | 2 | 2 |
| R8 | Go toolchain (the only dependency) is not pinned, not scanned, and downloads itself automatically | Dependency | Plausible | 2 | 2 | 4 |
| R9 | No automated gate: every AGENTS.md §7 check depends on the agent's own report | Agent ops | Confirmed | 2 | 3 | 6 |
| R10 | Evidence can't be re-run, and the AC5 transcript is timing-dependent and older than the change | Agent ops | Confirmed | 3 | 2 | 6 |
| R11 | The "refactor" commit also changes behaviour, against AGENTS.md §8.1 | Agent ops | Confirmed | 3 | 2 | 6 |
| R12 | The rules reach agents in a fragile way; this review got a placeholder | Agent ops | Confirmed (seen in this review) | 2 | 2 | 4 |

L = likelihood (1–3), I = impact (1–3).

---

### R1. A line over 64 KiB ends the session and loses every task (Correctness)
- **Cause:** `CLI.Run` uses `bufio.NewScanner` with its default 64 KiB line limit. The whole `add … | description` command must fit on one line.
- **Event:** The user pastes a long description. The app's main job is analysing description text.
- **Consequence:** `Run` returns `bufio.ErrTooLong`, `main` exits with code 1, and all in-memory tasks are gone.
- **Component:** `internal/cli/cli.go`, `cmd/taskflow/main.go`
- **Evidence:**
  - `internal/cli/cli.go:78` `scanner := bufio.NewScanner(in)`; `:92` `return scanner.Err()`
  - `cmd/taskflow/main.go:49` `return cli.New(svc, os.Stdout, clock).Run(os.Stdin)`; `:16-18` `fmt.Fprintln(os.Stderr, "ошибка:", err)` then `os.Exit(1)`
  - Go 1.24.7 standard library, `/usr/local/go1.24.7/src/bufio/scan.go:82`: `MaxScanTokenSize = 64 * 1024`
  - Baseline probe (cfa20b2, not re-run by me), `02_baseline_probes.txt:2-3`: `Run returned error: bufio.Scanner: token too long` and `command after the long line executed: false`. The reviewed commit did not touch the scanner lines.
- **Confidence:** Confirmed. The code and the stdlib constant make it certain; I did not run it end-to-end.
- **L=2:** Pasting long text is a normal use of an analysis tool. **I=3:** The whole session is lost and there is no persistence.
- **Mitigation:** Read input with `bufio.Reader` (or give the scanner a larger buffer). When a line is too long, print an error in Russian and keep reading. Add a `MaxDescriptionLen` check in `task.New`.
- **Verify:** Add a CLI test that runs `"add x | "+strings.Repeat("а", 70*1024)+"\nadd second\n"`. It must get a `nil` error back and output containing `создана задача`. Then run `go test -count=1 -race ./internal/cli`.

### R2. Any line starting with `exit` or `quit` ends the session (Correctness, introduced by f07c483)
- **Cause:** `Execute` lowercases the first word and looks it up in the command table. The exit entry ignores its arguments.
- **Event:** A line such as `Quit smoking by Friday`, `EXIT`, or `exit 3` is typed or pasted. When multi-line text is pasted after `add … |`, each later line runs as a command.
- **Consequence:** The session ends and every task is lost. Before this commit only an exact `exit` or `quit` line did that.
- **Component:** `internal/cli/cli.go`
- **Evidence:**
  - `cli.go:51-52` `{names: []string{"exit", "quit"}, … run: func(*CLI, []string, string) error { return ErrExit }}`
  - `cli.go:105-106` `name := strings.ToLower(fields[0])` / `cmd, ok := c.byName[name]`
  - The diff `8db77b3..f07c483` removes the exact match `if line == "exit" || line == "quit" {`
  - `commands_test.go:54` now expects `"EXIT"` to exit.
- **Confidence:** Confirmed from the code and the diff.
- **L=1:** Most text is Russian, so few lines start with these English words. **I=3:** Total data loss.
- **Mitigation:** Make exit refuse arguments, for example `if len(args) > 0 { return errors.New("exit не принимает аргументов") }`. Decide with you whether `EXIT` in capitals should exit. This is a behaviour change, so per AGENTS.md it needs its own change and a test that fails first.
- **Verify:** Add a test that `Execute("quit smoking")` does not return `ErrExit`. `TestCLI_ExitStopsReading` must still pass.

### R3. A panic in a processor crashes the whole process (Correctness)
- **Cause:** There is no `recover` in `Pool.run` or `Service.process`.
- **Event:** A processor panics. The `Processor` interface is open for new processors; `task.Analyze` itself has no input that makes it panic.
- **Consequence:** The process dies with all data. If someone adds a bare `recover`, the task instead stays stuck. A queued task can only move to in_progress or todo, and an in_progress task only to done or failed. Delete refuses busy tasks. So the stuck task can't be cancelled, deleted or resubmitted.
- **Component:** `internal/service/service.go`, `internal/worker/pool.go`
- **Evidence:**
  - `service.go:129` `result, procErr := s.proc.Process(t)`
  - `pool.go:56-57` `for id := range p.queue { p.handle(worker, id) }`
  - `git grep "recover()"` returns nothing.
  - `status.go:22-23` `StatusQueued: {StatusInProgress, StatusTodo}`, `StatusInProgress: {StatusDone, StatusFailed}`
  - `memory.go:71-72` `if t.Status().IsBusy() { return … ErrTaskBusy`
  - The second save is only logged: `service.go:137-138` `log.Printf("worker %d: save task %d: %v", …)`
  - Baseline probe `02:16-28` shows `panic: unexpected input in processor` and `[exit code: 2]` (cfa20b2; this code is unchanged since).
- **Confidence:** Confirmed for the crash. The stuck-task part is Speculative, since no current code path reaches it.
- **L=1, I=3.**
- **Mitigation:** In `Service.process`, add a deferred `recover` that moves the task to `failed` with a reason like "processor panic". Do not touch locking or the shutdown order (AGENTS.md §5.6).
- **Verify:** A service test with a processor that panics on one task. That task must end `failed`, the other tasks `done`, and the test process must survive. Run with `-race`.

### R4. Deadline "end of day" is wrong on daylight-saving days (Correctness)
- **Cause:** `parseDate` adds a fixed duration of 24h minus 1s to local midnight.
- **Event:** The deadline falls on a daylight-saving switch day in the user's time zone.
- **Consequence:** On a 23-hour day the deadline becomes 00:59:59 the next day. For example, in Europe/Berlin `@2027-03-28` would show `29.03.2027`. On a 25-hour day the task is marked overdue an hour early.
- **Component:** `internal/cli/cli.go`
- **Evidence:** `cli.go:325` `return day.Add(24*time.Hour - time.Second), nil`. `time.Add` adds an absolute duration (`time.go:1165` "Add returns the time t+d"). The tests use UTC only (`cli_test.go:21`).
- **Confidence:** Confirmed from the code. Not executed.
- **L=1:** Kazakhstan has no daylight saving. **I=1:** A wrong date shown, or an overdue mark one hour early.
- **Mitigation:** `time.Date(y, m, d, 23, 59, 59, 0, loc)`.
- **Verify:** A CLI test with a fixed clock in Europe/Berlin (import `time/tzdata`, which is standard library) runs `add x @2027-03-28` and `show 1`. It must expect `дедлайн:   28.03.2027`.

### R5. The documented package map is already wrong, and boundaries are not checked (Maintainability)
- **Cause:** `cli` imports `worker` to map its errors, but AGENTS.md lists the allowed directions as `cli → service → task`, `service → worker`, `storage → task`.
- **Event:** An agent trusts the map. It either "fixes" the code (which changes behaviour) or adds more undocumented imports. Nothing checks the rule that "storage and worker must never import service or cli".
- **Consequence:** Package boundaries erode without anyone noticing, and documentation and code keep drifting apart.
- **Component:** `internal/cli/cli.go`, `AGENTS.md`
- **Evidence:**
  - `cli.go:14` `"taskflow/internal/worker"`; `cli.go:353-356` `errors.Is(err, worker.ErrQueueFull)` and `worker.ErrStopped`
  - `AGENTS.md:25` `Dependency direction: \`cli → service → task\`, \`service → worker\`, \`storage → task\`.`
  - No test inspects imports. `git grep` lists the import edges, and that check is manual.
- **Confidence:** Confirmed.
- **L=2, I=2.**
- **Mitigation:** Either have `service` export these errors and drop the `cli → worker` import, or document that edge in AGENTS.md. Add a small import-boundary test that uses `go/parser` from the standard library.
- **Verify:** The new test fails if `internal/storage` or `internal/worker` imports `service` or `cli`. Also run `git grep -n '"taskflow/internal/\(service\|cli\)"' -- internal/storage internal/worker`, which must print nothing.

### R6. The new command-table test checks the table against itself (Maintainability)
- **Cause:** `TestCommandTable_HelpAndDispatchAgree` compares `commandTable()` with the help output and with `byName`, and both of those are built from the same table. Nothing pins the AC5 command list on its own.
- **Event:** A command or its success message is renamed or dropped.
- **Consequence:** The test still passes, and AC5 ("messages documented in README keep working") could regress without any test failing.
- **Component:** `internal/cli/commands_test.go`, `internal/cli/cli_test.go`
- **Evidence:**
  - `commands_test.go:33` `for _, cmd := range commandTable() {`; `:42` `strings.Contains(help, name)`; `:45` `c.byName[name]`
  - `cli.go:68-72` builds `byName` from `commandTable()`.
  - `git show a05-reviewed:internal/cli/cli_test.go | grep "удалена"` finds nothing. The delete success message (`cli.go:263` `"задача #%d удалена\n"`) and a successful `reopen` are only tested at the service level.
- **Confidence:** Confirmed.
- **L=2, I=2.**
- **Mitigation:** Add a test with a hard-coded list of the 11 AC5 names plus `quit`, checked against the table both ways. Add CLI asserts for `задача #1 удалена` and for `reopen` → `задача #N: todo`.
- **Verify:** On a scratch branch, delete the `delete` success `Fprintf`. The new test must fail.

### R7. User text is printed raw (Security, low)
- **Cause:** `task.New` only trims spaces. There is no control-character check and no description size limit. The description keeps everything after `|`, including a carriage return in the middle of the line; the title keeps ESC characters.
- **Event:** Untrusted text is pasted or piped in. The README and the evidence both use `printf … | go run`.
- **Consequence:** Terminal escape sequences can rewrite what is shown, for example making a failed task look done, in the terminal and in the transcripts committed as evidence. A 10 MB description is accepted at the service level.
- **Component:** `internal/task/task.go`, `internal/cli/cli.go`
- **Evidence:**
  - `task.go:41` `title := strings.TrimSpace(p.Title)`; `task.go:66` `Description: strings.TrimSpace(p.Description)`
  - `cli.go:122` `strings.Cut(rest, "|")`; `cli.go:223` `fmt.Fprintf(c.out, "  описание:  %s\n", t.Description)`; `cli.go:300` prints the title.
  - `git grep "IsControl\|IsPrint\|IsGraphic"` finds nothing.
  - Baseline probe `02:11` `description of 9999999 bytes -> err: <nil>`.
- **Confidence:** Plausible. Not executed.
- **L=1:** Single local user. **I=2:** Misleading output and evidence.
- **Mitigation:** In `task.New`, reject `unicode.IsControl` characters other than tab in the title, description and tags, and set `MaxDescriptionLen`. Both return a `ValidationError`.
- **Verify:** Table tests in `task_test.go` for `"\x1b[2K"`, a carriage return in the middle of text, and a description one rune over the limit.

### R8. The Go toolchain is not pinned, not scanned, and downloads itself (Dependency)
- **Cause:** `go.mod` declares only the minimum `go 1.22` with no `toolchain` line, and `GOTOOLCHAIN=auto` is set. Vulnerability checks have never run.
- **Event:** Contributors and agents use different Go versions (the local one is 1.24.7). Or someone raises the `go` line, and a "safe" `go build` or `go test` silently downloads and runs a new toolchain.
- **Consequence:**
  - Vet and test results differ between machines.
  - Vulnerabilities in the standard library go unnoticed.
  - A "safe" command reaches the network.
  - By Go's release policy (a release is supported until two newer ones exist), 1.24 is probably out of support. I could not check that online here.
- **Component:** `go.mod`, the development environment
- **Evidence:**
  - `go.mod:3` `go 1.22`; `git show a05-reviewed:go.mod | grep toolchain` finds nothing.
  - `go env GOTOOLCHAIN` prints `auto`; `go version` prints `go1.24.7 linux/amd64`.
  - `01_baseline_checks.txt:53-54` `govulncheck … NOT EXECUTED … proxy.golang.org blocked`.
- **Confidence:** Plausible. The download and the support status are not verified.
- **L=2, I=2.**
- **Mitigation:** Pin the Go version in CI (for example `go-version-file: go.mod` plus an exact patch release) and run `govulncheck ./...` there. Tell agents to use `GOTOOLCHAIN=local`. Any `toolchain` line in `go.mod` needs your review (AGENTS.md §5.4).
- **Verify:** In the agent's environment, `go env GOTOOLCHAIN` prints `local`. The CI log shows the exact Go version and the govulncheck result.

### R9. No automated gate (Agent ops)
- **Cause:** There is no CI workflow, Makefile or hook. AGENTS.md §7 and §8 rely on the agent reporting its own results and a human re-running them.
- **Event:** An agent skips or misreports `-race`, or can't run it at all. For example, `-race` needs cgo, which on Windows/GoLand setups means a C compiler.
- **Consequence:** A data race could be merged, breaking AC4, or a regression could slip through.
- **Component:** Repository root
- **Evidence:**
  - `git ls-tree -r --name-only a05-reviewed | grep -i "probe\|\.github\|Makefile\|golangci\|pre-commit"` only matches `docs/assignment05/evidence/02_baseline_probes.txt`, which is not a gate.
  - `AGENTS.md:99` "A human reads the whole diff and re-runs the checks".
  - Run without `-count=1`, the README's `go test ./...` (`README.md:127`) can show cached results.
- **Confidence:** Confirmed.
- **L=2, I=3:** Concurrency is the core of this project.
- **Mitigation:** Add a Linux CI job that runs the six AGENTS.md commands, failing on any `gofmt -l` output, and require it to pass before merging to `main`.
- **Verify:** On a scratch branch, an unformatted file and an injected data race must each turn CI red.

### R10. Evidence can't be re-run, and the AC5 transcript is stale (Agent ops)
- **Cause:** The probe code behind the evidence files was never committed. AC5's scripted run was captured before the CLI change, and its output depends on goroutine scheduling.
- **Event:** A reviewer tries to re-run a probe (AGENTS.md §8.3) or check AC5 at `a05-reviewed`.
- **Consequence:** The claims can't be reproduced; AC5 is only checked against an older, timing-dependent transcript; and a hand edit to an evidence file (forbidden by AGENTS.md §5.5) would not be detected.
- **Component:** `docs/assignment05/evidence/`, `docs/assignment05/acceptance-criteria.md`
- **Evidence:**
  - `02:1` `-run TestProbe_LongLineEndsSession` and `04:1` `TestProbe_HelpAndDispatchDrift`: no such test is in the tree.
  - `02:15` `./cmd/panicprobe` and `02:21` `/home/claude/a05_probe/...` point to a different checkout.
  - `01:1` `# commit: cfa20b2`.
  - `acceptance-criteria.md:9` AC5 is "Verified by … scripted run in … 03_baseline_run.txt".
  - In `03:16-20`, `list` shows all four tasks as `queued`, then `03:22` `show 1` shows `done`, with a worker number that varies.
  - The post-change transcript (`05_pattern_after.txt`) exists only after the tag, in commit 25a70cc.
- **Confidence:** Confirmed.
- **L=3:** It has already happened. **I=2.**
- **Mitigation:**
  - Commit the probes as real regression tests. R1's probe should become a test.
  - Replace the AC5 transcript with a Go test that runs the documented script and calls `Shutdown` before `list`.
  - Generate the evidence with a committed script whose header records the commit hash.
- **Verify:** On a clean clone, run the script, then `git diff --exit-code docs/assignment05/evidence`.

### R11. The "refactor" commit also changes behaviour (Agent ops)
- **Cause:** f07c483 is titled "Refactor CLI" but also changes how exit works (any case, with arguments, and now through `Execute`), changes the help layout, and adds an evidence file. AGENTS.md:96 says "never mix refactoring with behaviour changes".
- **Event:** The reviewer reads it as a pure refactor.
- **Consequence:** Behaviour changes slip past review; R2 is the concrete result. The test was added in the same commit, so the history doesn't show it failing first (AGENTS.md:89).
- **Component:** `internal/cli/cli.go`, the review process
- **Evidence:**
  - `git diff 8db77b3 f07c483 -- internal/cli/cli.go` removes `if line == "exit" || line == "quit" {` and the fixed `helpText`, which had the line `cancel <id> | reopen <id>`.
  - `commands_test.go:54` `[]string{"exit", "quit", "EXIT"}`.
  - `git show --stat f07c483` also lists `evidence/04_pattern_before.txt | 30 ++++++`.
- **Confidence:** Confirmed.
- **L=3, I=2.**
- **Mitigation:** Split future changes into a pure refactor, protected by a golden test of the old help output, and a separate behaviour change with its own failing-first test. Record the exit-semantics decision.
- **Verify:** The golden help test passes on the refactor commit by itself, and `git log` shows separate commits or branches.

### R12. The rules reach agents in a fragile way (Agent ops)
- **Cause:** Only tools that read `CLAUDE.md` get AGENTS.md automatically (`CLAUDE.md:1` `@AGENTS.md`). Other agents and subagents get it pasted in through a template, and nothing checks that the paste worked.
- **Event:** The template fails to fill in the file. It happened in this review: the prompt contained the literal `{AGENTS}` until you corrected it partway through.
- **Consequence:** An agent works without the forbidden-actions list (tags, pushes, editing evidence, dependency changes), and nothing would notice.
- **Component:** `AGENTS.md`, `CLAUDE.md`, the orchestration around them
- **Evidence:** This session's prompt (seen directly). `CLAUDE.md:1` `@AGENTS.md`.
- **Confidence:** Confirmed (seen in this review).
- **L=2, I=2.**
- **Mitigation:**
  - Have each agent read `git show <ref>:AGENTS.md` itself and report `git hash-object AGENTS.md`.
  - Refuse to dispatch any prompt that still contains an unfilled `{…}` placeholder.
- **Verify:** `grep -n '{AGENTS}'` on the final prompt finds nothing, and the hash in the agent's report matches the one in the repo.

---

## Top-2 recommendations

1. **Stop console input from ending the session (fix R1 and R2 together).** These are the only candidates where a normal action (a long paste, or a line starting with `quit`) destroys all your data. Nothing is saved to disk, so nothing can be recovered. R2 was added by the reviewed commit itself.
   - Read input with `bufio.Reader` plus a size check, reject over-long lines without ending the session, and make `exit`/`quit` refuse arguments.
   - Two small CLI tests prove it: a 70 KiB line followed by `add second`, and `Execute("quit smoking")`.
   - Do this as a separate behaviour change with failing-first tests, per AGENTS.md §7 and §8.1.

2. **Make the gates automatic and the evidence re-runnable (fix R9 and R10, which also covers most of R8).** Every other fix depends on checks that today run only if an agent says it ran them.
   - A Linux CI job with a pinned Go version: gofmt, vet, build, `-race`, cover, and govulncheck wherever the network is allowed.
   - The probes committed as real regression tests.
   - Together these turn AC4 and AC5 into enforced checks, and stop relying on self-reports and an old, timing-dependent transcript.
