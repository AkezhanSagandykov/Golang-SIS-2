# Prompt sent to the read-only review agent (Task 4)

- Date: 2026-11-21, about 18:30 (UTC+5)
- Tool: Claude sub-agent of type "Plan" (no file-editing tools), started by the main Claude assistant in claude.ai.
  Model: Claude Opus 5.5 (same model as the main assistant).
- Disclosure: the agent does not load `AGENTS.md` automatically. The prompt was meant to contain its full text,
  but by mistake it contained the literal placeholder `{AGENTS}`. A follow-up message with the full text was sent
  while the agent was working; the agent also read `AGENTS.md` from the tag itself and confirmed the two were identical
  (see `review-output.md`). This failure is recorded as risk R12.

## Prompt (verbatim)

```text
You are an independent risk reviewer. This is a READ-ONLY review: do not create, edit, move or delete any file; do not run git commands that change state (no commit, checkout, reset, stash, tag, branch); do not install anything; do not use the network. You may read files and run read-only commands: `git -C <repo> show/diff/log`, `cat`, `grep`, `ls`, and the safe commands listed in AGENTS.md below (`gofmt -l .`, `go vet ./...`, `go build ./...`, `go test -count=1 ./...`, `go test -count=1 -race ./...`, `go test -count=1 -cover ./...`). Note: `go build` and `go test` write only to Go's cache, which is allowed.

Repository: /home/claude/akezhansagandykov/golang-sis-2 (Go module `taskflow`). Review the code exactly as of git tag `a05-reviewed` (commit f07c483). Read files with `git -C /home/claude/akezhansagandykov/golang-sis-2 show a05-reviewed:<path>`; list them with `git -C ... ls-tree -r --name-only a05-reviewed`. Ignore everything under docs/assignment05/evidence except as context.

Project context: a small console task tracker. Users add tasks; `submit` sends a task to a fixed worker pool that analyzes the task description (word count, unique words, keywords). Data is in memory only. No network, no database, no deployment.

The repository's agent instructions (AGENTS.md) are pasted here because your tool does not load them automatically. Follow them:
-----BEGIN AGENTS.md-----
{AGENTS}
-----END AGENTS.md-----

Acceptance criteria that must stay true: docs/assignment05/acceptance-criteria.md (read it).

Task: propose candidate RISKS for this version. Cover at least these categories, with at least one candidate each where the code gives you a reason: correctness, maintainability, security, dependency, agent operations (how safely an AI agent can work in this repo: instructions, gates, reproducibility, CI). Give 8–12 candidates. For each candidate:
- ID (R1, R2, ...), short title, category
- Cause → event → consequence (one line each)
- Affected component (file/package)
- Evidence: file path + line numbers at a05-reviewed with a short code excerpt, or the exact command you ran and its real output. If you did not run something, say "not executed".
- Your confidence: Confirmed (you reproduced it or the code makes it certain), Plausible (likely but not demonstrated), or Speculative
- Suggested likelihood (1–3) and impact (1–3) with one-line reasoning
- A minimal mitigation and how to verify it (a test or command)

Rules: do not invent command output or line numbers; mark anything you did not verify. Do not pad the list with style nits. Return the result as markdown text in your final message, ending with your top-2 recommendations for corrective action and why.
```

## Follow-up message (summary)

"Correction to your instructions: the AGENTS.md block in your prompt contained a placeholder ({AGENTS}) instead of
the file. Here is the full text; follow it (it is identical to `git show a05-reviewed:AGENTS.md`): …" followed by the
full content of AGENTS.md sections 1–8, and "Your review remains read-only. Continue with the task as described."

Note: `f07c483` and `a05-reviewed` refer to the same commit; after the trailer amendment it is `275c985`
(see `../evidence/README.md`).
