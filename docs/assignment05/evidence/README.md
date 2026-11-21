# Evidence files — how to read them

Every `.txt` file here is real command output, saved by a script and not edited by hand.
Probe tests used for evidence were run in a separate worktree and are not part of the history.
The two most important probes (long line, `quit …`) became permanent regression tests in `internal/cli`.

## Commit hashes in the logs

Before the branch was published, every commit on it was amended once, only to add AI co-author
trailers (`Co-Authored-By: Claude …`). File contents did not change (the final tree hash is identical),
but the commit hashes did. The logs were captured before that, so they show the old hashes:

| Old hash (in logs) | New hash | Commit |
|---|---|---|
| `8db77b3` | `4b442d0` | AGENTS.md, CLAUDE.md, acceptance criteria, baseline evidence |
| `f07c483` | `275c985` | Refactor CLI: command table (tag `a05-reviewed`) |
| `25a70cc` | `deee43e` | Evidence after the refactoring |
| `0333ce3` | `d46d787` | Mitigation M1 (R1): over-long input lines |
| `967ef30` | `dde47a4` | Mitigation M1 (R2): exit/quit with arguments |
| `1d0532e` | `2c6fba3` | Mitigation M2 (R9): quality gate + CI |
| `d7e8e96` | `6c61ebd` | Evidence: review verification, final checks, final probes |

The baseline `cfa20b2` (tag `a05-baseline`) was not changed.

## Files

| File | What it shows | Commit |
|---|---|---|
| `01_baseline_checks.txt` | gofmt, vet, build, tests, race, cover at the baseline | `cfa20b2` |
| `02_baseline_probes.txt` | W1 long line, W2 processor panic, W4 description size, missing gates | `cfa20b2` |
| `03_baseline_run.txt` | README demo session, `-workers 1`, full queue | `cfa20b2` |
| `04_pattern_before.txt` | help / dispatch drift before the refactoring | `cfa20b2` |
| `05_pattern_after.txt` | all checks after the refactoring | `f07c483` → `275c985` |
| `05b_review_verification_at_a05-reviewed.txt` | probes that verify the AI review findings R1, R2, R4, R7 | `f07c483` → `275c985` |
| `06_m1_r1_long_line.txt` | R1 test failing first, then passing | `0333ce3` → `d46d787` |
| `07_m1_r2_exit_args.txt` | R2 test failing first, then passing | `967ef30` → `dde47a4` |
| `08_m2_gate_demo.txt` | the gate fails on bad formatting and on a data race, passes when clean | `967ef30` + uncommitted M2 files |
| `09_final_checks.txt` | all checks + scripted session at the final code | `1d0532e` → `2c6fba3` |
| `10_final_probes.txt` | review probes re-run at the final code | `1d0532e` → `2c6fba3` |
