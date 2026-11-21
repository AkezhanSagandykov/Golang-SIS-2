package task_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"taskflow/internal/task"
)

var now = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

func mustNew(t *testing.T, p task.Params) task.Task {
	t.Helper()
	tk, err := task.New(p, now)
	if err != nil {
		t.Fatalf("task.New(%+v): %v", p, err)
	}
	return tk
}

func TestNew_Defaults(t *testing.T) {
	tk := mustNew(t, task.Params{
		Title:       "  Сдать Checkpoint 2  ",
		Description: " worker pool + tests ",
		Tags:        []string{"#Go", "go", " Study ", "", "#"},
	})

	if tk.Title != "Сдать Checkpoint 2" {
		t.Errorf("Title = %q, want trimmed", tk.Title)
	}
	if tk.Description != "worker pool + tests" {
		t.Errorf("Description = %q, want trimmed", tk.Description)
	}
	if tk.Status() != task.StatusTodo {
		t.Errorf("Status = %s, want todo", tk.Status())
	}
	if tk.Priority != task.PriorityMedium {
		t.Errorf("Priority = %s, want medium (default)", tk.Priority)
	}
	if got := strings.Join(tk.Tags, ","); got != "go,study" {
		t.Errorf("Tags = %q, want \"go,study\"", got)
	}
	if !tk.CreatedAt.Equal(now) || !tk.UpdatedAt.Equal(now) {
		t.Errorf("CreatedAt/UpdatedAt = %v/%v, want %v", tk.CreatedAt, tk.UpdatedAt, now)
	}
	if tk.ID != 0 || tk.Worker != 0 || tk.Result.Words != 0 {
		t.Errorf("new task must have no ID, worker or result: %+v", tk)
	}
}

func TestNew_OnlyEmptyTagsGiveNil(t *testing.T) {
	tk := mustNew(t, task.Params{Title: "x", Tags: []string{"", "  #  "}})
	if tk.Tags != nil {
		t.Fatalf("Tags = %#v, want nil", tk.Tags)
	}
}

func TestNew_DoesNotKeepCallerSlice(t *testing.T) {
	tags := []string{"go"}
	tk := mustNew(t, task.Params{Title: "x", Tags: tags})
	tags[0] = "changed"
	if tk.Tags[0] != "go" {
		t.Fatalf("task shares Tags with the caller: %v", tk.Tags)
	}
}

func TestNew_Validation(t *testing.T) {
	tests := []struct {
		name      string
		params    task.Params
		wantField string
	}{
		{"empty title", task.Params{Title: ""}, "title"},
		{"spaces only", task.Params{Title: "   "}, "title"},
		{"title too long", task.Params{Title: strings.Repeat("я", task.MaxTitleLen+1)}, "title"},
		{"unknown priority", task.Params{Title: "x", Priority: 10}, "priority"},
		{"deadline in the past", task.Params{Title: "x", Deadline: now.Add(-time.Minute)}, "deadline"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk, err := task.New(tt.params, now)

			if !errors.Is(err, task.ErrValidation) {
				t.Fatalf("err = %v, want ErrValidation", err)
			}
			var ve *task.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err %T is not *ValidationError", err)
			}
			if ve.Field != tt.wantField {
				t.Errorf("Field = %q, want %q", ve.Field, tt.wantField)
			}
			if tk.Title != "" || tk.Status() != 0 {
				t.Errorf("on error New must return zero Task, got %+v", tk)
			}
		})
	}
}

func TestNew_TitleLengthCountsRunesNotBytes(t *testing.T) {
	mustNew(t, task.Params{Title: strings.Repeat("я", task.MaxTitleLen)})
}

func TestLifecycle_Success(t *testing.T) {
	tk := mustNew(t, task.Params{Title: "x", Description: "Hello Go world"})
	result, _ := task.Analyze(tk.Description)

	if err := tk.TransitionTo(task.StatusQueued, now.Add(time.Minute)); err != nil {
		t.Fatalf("todo -> queued: %v", err)
	}
	if err := tk.Start(2, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("queued -> in_progress: %v", err)
	}
	if tk.Status() != task.StatusInProgress || tk.Worker != 2 {
		t.Fatalf("after Start: status=%s worker=%d", tk.Status(), tk.Worker)
	}
	if err := tk.Complete(result, now.Add(3*time.Minute)); err != nil {
		t.Fatalf("in_progress -> done: %v", err)
	}

	if tk.Status() != task.StatusDone {
		t.Errorf("Status = %s, want done", tk.Status())
	}
	if tk.Result.Words != 3 || !slices.Equal(tk.Result.Keywords, []string{"hello", "world"}) {
		t.Errorf("Result = %+v", tk.Result)
	}
	if !tk.UpdatedAt.Equal(now.Add(3 * time.Minute)) {
		t.Errorf("UpdatedAt = %v", tk.UpdatedAt)
	}
}

func TestLifecycle_FailureAndReopen(t *testing.T) {
	tk := mustNew(t, task.Params{Title: "x"})
	moveTo(t, &tk, task.StatusInProgress)

	if err := tk.Fail("nothing to analyze", now); err != nil {
		t.Fatalf("in_progress -> failed: %v", err)
	}
	if tk.Status() != task.StatusFailed || tk.FailReason != "nothing to analyze" {
		t.Fatalf("after Fail: status=%s reason=%q", tk.Status(), tk.FailReason)
	}
	if tk.Result.Words != 0 || tk.Result.Keywords != nil {
		t.Errorf("failed task must not have a result: %+v", tk.Result)
	}

	if err := tk.Reopen(now); err != nil {
		t.Fatalf("failed -> todo: %v", err)
	}
	if tk.Status() != task.StatusTodo {
		t.Errorf("Status after Reopen = %s, want todo", tk.Status())
	}
}

func TestReopen_OnlyFromFailedOrCancelled(t *testing.T) {
	for _, status := range task.AllStatuses() {
		t.Run(status.String(), func(t *testing.T) {
			tk := mustNew(t, task.Params{Title: "x"})
			moveTo(t, &tk, status)

			err := tk.Reopen(now)

			canReopen := status == task.StatusFailed || status == task.StatusCancelled
			if canReopen && err != nil {
				t.Fatalf("Reopen from %s: %v", status, err)
			}
			if !canReopen && !errors.Is(err, task.ErrInvalidTransition) {
				t.Fatalf("Reopen from %s: err = %v, want ErrInvalidTransition", status, err)
			}
		})
	}
}

func TestCompleteAndStart_RequireRightStatus(t *testing.T) {
	tk := mustNew(t, task.Params{Title: "x"})

	if err := tk.Start(1, now); !errors.Is(err, task.ErrInvalidTransition) {
		t.Errorf("Start from todo: err = %v, want ErrInvalidTransition", err)
	}
	if err := tk.Complete(task.Result{Words: 5}, now); !errors.Is(err, task.ErrInvalidTransition) {
		t.Errorf("Complete from todo: err = %v, want ErrInvalidTransition", err)
	}
	if tk.Status() != task.StatusTodo || tk.Worker != 0 || tk.Result.Words != 0 {
		t.Errorf("failed calls changed the task: %+v", tk)
	}
}

func TestTransitionTo_InvalidLeavesTaskUnchanged(t *testing.T) {
	tk := mustNew(t, task.Params{Title: "x"})
	tk.ID = 7

	err := tk.TransitionTo(task.StatusDone, now.Add(time.Hour))

	var te *task.TransitionError
	if !errors.As(err, &te) {
		t.Fatalf("err %T is not *TransitionError", err)
	}
	if te.ID != 7 || te.From != task.StatusTodo || te.To != task.StatusDone {
		t.Errorf("TransitionError = %+v", *te)
	}
	if !errors.Is(err, task.ErrInvalidTransition) {
		t.Errorf("errors.Is(err, ErrInvalidTransition) = false")
	}
	if tk.Status() != task.StatusTodo || !tk.UpdatedAt.Equal(now) {
		t.Errorf("task changed after failed transition: status=%s updated=%v", tk.Status(), tk.UpdatedAt)
	}
	if got := err.Error(); got != "task 7: todo -> done: invalid status transition" {
		t.Errorf("Error() = %q", got)
	}
}

func TestTransitionTo_FromDoneIsImpossible(t *testing.T) {
	tk := mustNew(t, task.Params{Title: "x"})
	moveTo(t, &tk, task.StatusDone)

	for _, to := range task.AllStatuses() {
		if err := tk.TransitionTo(to, now); !errors.Is(err, task.ErrInvalidTransition) {
			t.Errorf("done -> %s: err = %v, want ErrInvalidTransition", to, err)
		}
	}
}

func TestTransitionTo_UnknownTargetStatus(t *testing.T) {
	tk := mustNew(t, task.Params{Title: "x"})
	if err := tk.TransitionTo(task.Status(42), now); !errors.Is(err, task.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

func TestZeroValueTaskCannotTransition(t *testing.T) {
	var tk task.Task
	if err := tk.TransitionTo(task.StatusQueued, now); !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestIsOverdue(t *testing.T) {
	deadline := now.Add(24 * time.Hour)
	tests := []struct {
		name     string
		deadline time.Time
		status   task.Status
		at       time.Time
		want     bool
	}{
		{"no deadline", time.Time{}, task.StatusTodo, now.AddDate(1, 0, 0), false},
		{"before deadline", deadline, task.StatusTodo, deadline.Add(-time.Second), false},
		{"exactly at deadline", deadline, task.StatusTodo, deadline, false},
		{"after deadline, todo", deadline, task.StatusTodo, deadline.Add(time.Second), true},
		{"after deadline, queued", deadline, task.StatusQueued, deadline.Add(time.Hour), true},
		{"after deadline, failed", deadline, task.StatusFailed, deadline.Add(time.Hour), true},
		{"after deadline, done", deadline, task.StatusDone, deadline.Add(time.Hour), false},
		{"after deadline, cancelled", deadline, task.StatusCancelled, deadline.Add(time.Hour), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := mustNew(t, task.Params{Title: "x", Deadline: tt.deadline})
			moveTo(t, &tk, tt.status)
			if got := tk.IsOverdue(tt.at); got != tt.want {
				t.Errorf("IsOverdue = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasTag(t *testing.T) {
	tk := mustNew(t, task.Params{Title: "x", Tags: []string{"Go", "study"}})
	for _, tag := range []string{"go", "#GO", " study "} {
		if !tk.HasTag(tag) {
			t.Errorf("HasTag(%q) = false, want true", tag)
		}
	}
	if tk.HasTag("java") {
		t.Error("HasTag(java) = true, want false")
	}
}

func TestClone_IsIndependent(t *testing.T) {
	original := mustNew(t, task.Params{Title: "x", Tags: []string{"a", "b"}})
	original.Result = task.Result{Words: 2, Keywords: []string{"alpha", "beta"}}

	shallow := original
	shallow.Tags[0] = "changed-by-shallow"
	if original.Tags[0] != "changed-by-shallow" {
		t.Fatal("expected a plain copy to share Tags (this is how Go slices work)")
	}

	deep := original.Clone()
	deep.Tags[1] = "changed-by-deep"
	deep.Result.Keywords[0] = "changed-by-deep"
	if original.Tags[1] != "b" || original.Result.Keywords[0] != "alpha" {
		t.Fatalf("Clone shares memory with original: tags=%v keywords=%v", original.Tags, original.Result.Keywords)
	}
}

func moveTo(t *testing.T, tk *task.Task, target task.Status) {
	t.Helper()
	path := map[task.Status][]task.Status{
		task.StatusTodo:       nil,
		task.StatusQueued:     {task.StatusQueued},
		task.StatusInProgress: {task.StatusQueued, task.StatusInProgress},
		task.StatusDone:       {task.StatusQueued, task.StatusInProgress, task.StatusDone},
		task.StatusFailed:     {task.StatusQueued, task.StatusInProgress, task.StatusFailed},
		task.StatusCancelled:  {task.StatusCancelled},
	}
	for _, s := range path[target] {
		if err := tk.TransitionTo(s, tk.UpdatedAt); err != nil {
			t.Fatalf("moveTo %s: %v", target, err)
		}
	}
}
