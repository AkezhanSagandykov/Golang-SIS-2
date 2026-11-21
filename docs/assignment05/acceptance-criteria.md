# Acceptance criteria (must stay true after every change)

| ID | Criterion | Verified by |
|---|---|---|
| AC1 | Task status changes only along the transition table in `internal/task/status.go`; any other change returns `ErrInvalidTransition` and leaves the task unchanged. | `internal/task/status_test.go`, `TestSubmit_RejectsInvalidStates` |
| AC2 | A submitted task is processed by exactly one worker and ends in `done` with its own analysis result, or in `failed` with a reason; results are never mixed between tasks. | `TestSubmit_ProcessesTasksAndStoresTheirOwnResults`, `TestSubmit_SameTaskFromManyGoroutines`, `TestSubmit_ProcessingFailureMarksTaskFailed` |
| AC3 | `submit` never blocks: when the queue is full or the pool is stopped, it returns an error and the task stays in `todo`. | `TestPool_RejectsWhenQueueFull`, `TestSubmit_QueueFullKeepsTaskInTodo`, `TestPool_SubmitAfterStopAndDoubleStop` |
| AC4 | `Shutdown` finishes every accepted task, rejects new ones, and all code is free of data races. | `TestShutdown_FinishesAcceptedTasksAndRejectsNewOnes`, `TestPool_StopFinishesAcceptedTasks`, `go test -race ./...` |
| AC5 | The CLI commands and messages documented in `README.md` keep working (`add`, `submit`, `list`, `show`, `cancel`, `reopen`, `delete`, `overdue`, `stats`, `help`, `exit`). | `internal/cli/cli_test.go`, scripted run in `docs/assignment05/evidence/03_baseline_run.txt` |
