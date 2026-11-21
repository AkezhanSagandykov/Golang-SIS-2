# AI-use record — Assignment #05

AI assistant used: **Claude (Anthropic)**, model **Claude Opus 5.5**, in claude.ai with code execution
(main assistant), plus one **read-only Claude sub-agent** of the same model for the risk review.
All work was done on 2026-11-21 (UTC+5).

| # | Tool / model | Prompt summary | Output used | Problems found | Changes made | Verification source |
|---|---|---|---|---|---|---|
| 1 | Claude Opus 5.5 (main) | Assignment PDF + "do Assignment #05 on my TaskFlow repo (Golang-SIS-2)" | Repository map, check script, probe tests, baseline evidence | staticcheck/govulncheck could not be downloaded (module proxy blocked) | Recorded as "not executed" | `evidence/01–03` |
| 2 | Claude Opus 5.5 (main) | Write AGENTS.md, CLAUDE.md and 3–5 acceptance criteria | `AGENTS.md`, `CLAUDE.md`, `acceptance-criteria.md` | Dependency map in AGENTS.md was wrong (`cli → worker` missing), found by the audit and by the reviewer (R5) | Fixed in the M2 commit | `go list` imports |
| 3 | Claude Opus 5.5 (main) | Evaluate Strategy / Command / Adapter for one maintainability problem; implement at most one | Command-table refactoring + 2 tests | It also changed exit behaviour (R11) and introduced R2 | R2 fixed in M1 with a failing-first test | `evidence/04, 05, 05b, 07` |
| 4 | Claude sub-agent (Plan, read-only) | `review-prompt.md` | 12 candidate risks (`review-output.md`) | Prompt carried a placeholder instead of AGENTS.md (R12); 2 sub-claims unsupported, 1 unverified | Follow-up message with AGENTS.md; each claim classified | `evidence/05b`, Go release sources |
| 5 | Claude Opus 5.5 (main) | Implement two minimal mitigations for the top confirmed risks with tests first | M1 (R1, R2), M2 (R9 gate + CI) | First line-limit diff allowed 2 bytes over the limit | Exact limit + `TestReadLine_LimitIsExact` | `evidence/06–10` |
| 6 | Claude Opus 5.5 (main) | Write the report (risk register, audit, safeguards, release decision, reflection) | Report document | Commits lacked AI co-author trailers | All branch commits amended once; hash map in `evidence/README.md` | final tree hash unchanged |

Human verification still required from the student before submission: re-run `./scripts/check.sh` on the
student's own machine, read every diff (`git log -p a05-baseline..a05-final`), push the branch and confirm the
GitHub Actions run of `checks.yml` is green.
