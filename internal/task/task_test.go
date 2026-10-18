package task_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"taskflow/internal/task"
)

var now = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

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
		Title:       "  Сдать Checkpoint 1  ",
		Description: " domain + tests ",
		Tags:        []string{"#Go", "go", " Study ", "", "#"},
	})

	if tk.Title != "Сдать Checkpoint 1" {
		t.Errorf("Title = %q, want trimmed", tk.Title)
	}
	if tk.Description != "domain + tests" {
		t.Errorf("Description = %q, want trimmed", tk.Description)
	}
	if tk.Status() != task.StatusTodo {
		t.Errorf("Status = %s, want todo", tk.Status())
	}
	if tk.Priority != task.PriorityMedium {
		t.Errorf("Priority = %s, want medium (default)", tk.Priority)
	}
	if got := strings.Join(tk.Tags, ","); got != "go,study" {
		t.Errorf("Tags = %q, want normalized and deduplicated \"go,study\"", got)
	}
	if !tk.CreatedAt.Equal(now) || !tk.UpdatedAt.Equal(now) {
		t.Errorf("CreatedAt/UpdatedAt = %v/%v, want %v", tk.CreatedAt, tk.UpdatedAt, now)
	}
	if tk.ID != 0 {
		t.Errorf("ID = %d, want 0 (ID assigns the repository)", tk.ID)
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
			if prefix := "validation failed: " + tt.wantField + " "; !strings.HasPrefix(err.Error(), prefix) {
				t.Errorf("Error() = %q, want prefix %q", err.Error(), prefix)
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

func TestTransitionTo_Lifecycle(t *testing.T) {
	tk := mustNew(t, task.Params{Title: "lifecycle"})
	steps := []task.Status{
		task.StatusInProgress,
		task.StatusTodo,
		task.StatusInProgress,
		task.StatusDone,
	}
	for i, to := range steps {
		at := now.Add(time.Duration(i+1) * time.Hour)
		if err := tk.TransitionTo(to, at); err != nil {
			t.Fatalf("step %d -> %s: %v", i, to, err)
		}
		if tk.Status() != to {
			t.Fatalf("step %d: status = %s, want %s", i, tk.Status(), to)
		}
		if !tk.UpdatedAt.Equal(at) {
			t.Errorf("step %d: UpdatedAt not updated", i)
		}
	}
}

func TestTransitionTo_InvalidLeavesTaskUnchanged(t *testing.T) {
	tk := mustNew(t, task.Params{Title: "x"})
	tk.ID = 7

	err := tk.TransitionTo(task.StatusDone, now.Add(time.Hour))

	if !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
	var te *task.TransitionError
	if !errors.As(err, &te) {
		t.Fatalf("err %T is not *TransitionError", err)
	}
	if te.ID != 7 || te.From != task.StatusTodo || te.To != task.StatusDone {
		t.Errorf("TransitionError = %+v", *te)
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
	_ = tk.TransitionTo(task.StatusInProgress, now)
	_ = tk.TransitionTo(task.StatusDone, now)

	for _, to := range task.AllStatuses() {
		if err := tk.TransitionTo(to, now); !errors.Is(err, task.ErrInvalidTransition) {
			t.Errorf("done -> %s: err = %v, want ErrInvalidTransition", to, err)
		}
	}
}

func TestTransitionTo_UnknownTargetStatus(t *testing.T) {
	tk := mustNew(t, task.Params{Title: "x"})
	err := tk.TransitionTo(task.Status(42), now)
	if !errors.Is(err, task.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

func TestZeroValueTaskCannotTransition(t *testing.T) {
	var tk task.Task
	err := tk.TransitionTo(task.StatusInProgress, now)
	if !errors.Is(err, task.ErrInvalidTransition) {
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
		{"after deadline, in progress", deadline, task.StatusInProgress, deadline.Add(time.Hour), true},
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

	shallow := original
	deep := original.Clone()

	shallow.Tags[0] = "changed-by-shallow"
	if original.Tags[0] != "changed-by-shallow" {
		t.Fatal("expected shallow copy to share Tags (this is how Go slices work)")
	}

	deep.Tags[1] = "changed-by-deep"
	if original.Tags[1] != "b" {
		t.Fatalf("Clone shares Tags with original: %v", original.Tags)
	}
}

func moveTo(t *testing.T, tk *task.Task, target task.Status) {
	t.Helper()
	path := map[task.Status][]task.Status{
		task.StatusTodo:       nil,
		task.StatusInProgress: {task.StatusInProgress},
		task.StatusDone:       {task.StatusInProgress, task.StatusDone},
		task.StatusCancelled:  {task.StatusCancelled},
	}
	for _, s := range path[target] {
		if err := tk.TransitionTo(s, tk.UpdatedAt); err != nil {
			t.Fatalf("moveTo %s: %v", target, err)
		}
	}
}
