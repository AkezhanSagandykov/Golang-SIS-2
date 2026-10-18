package task_test

import (
	"errors"
	"strings"
	"testing"

	"taskflow/internal/task"
)

func TestStatus_CanTransitionTo(t *testing.T) {
	type pair struct{ from, to task.Status }
	allowed := map[pair]bool{
		{task.StatusTodo, task.StatusInProgress}:      true,
		{task.StatusTodo, task.StatusCancelled}:       true,
		{task.StatusInProgress, task.StatusTodo}:      true,
		{task.StatusInProgress, task.StatusDone}:      true,
		{task.StatusInProgress, task.StatusCancelled}: true,
		{task.StatusCancelled, task.StatusTodo}:       true,
	}

	for _, from := range task.AllStatuses() {
		for _, to := range task.AllStatuses() {
			want := allowed[pair{from, to}]
			if got := from.CanTransitionTo(to); got != want {
				t.Errorf("%s -> %s: got %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestStatus_UnknownValues(t *testing.T) {
	for _, s := range []task.Status{0, 99, -1} {
		if s.IsValid() {
			t.Errorf("Status(%d).IsValid() = true, want false", int(s))
		}
		if s.CanTransitionTo(task.StatusTodo) {
			t.Errorf("Status(%d) must not transition anywhere", int(s))
		}
	}
	if got := task.Status(42).String(); got != "Status(42)" {
		t.Errorf("String() = %q, want %q", got, "Status(42)")
	}
}

func TestStatus_IsClosed(t *testing.T) {
	tests := map[task.Status]bool{
		task.StatusTodo:       false,
		task.StatusInProgress: false,
		task.StatusDone:       true,
		task.StatusCancelled:  true,
	}
	for status, want := range tests {
		if got := status.IsClosed(); got != want {
			t.Errorf("%s.IsClosed() = %v, want %v", status, got, want)
		}
	}
}

func TestParseStatus(t *testing.T) {
	tests := []struct {
		in      string
		want    task.Status
		wantErr bool
	}{
		{in: "todo", want: task.StatusTodo},
		{in: "In_Progress", want: task.StatusInProgress},
		{in: "  done ", want: task.StatusDone},
		{in: "cancelled", want: task.StatusCancelled},
		{in: "blocked", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := task.ParseStatus(tt.in)
			if tt.wantErr {
				if !errors.Is(err, task.ErrValidation) {
					t.Fatalf("err = %v, want ErrValidation", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
			if got.String() != tt.want.String() {
				t.Errorf("String round-trip broken: %q", got.String())
			}
		})
	}
}

func TestAllStatuses_ReturnsFreshSlice(t *testing.T) {
	first := task.AllStatuses()
	first[0] = 999
	if second := task.AllStatuses(); second[0] != task.StatusTodo {
		t.Fatalf("AllStatuses shares memory between calls: got %v", second[0])
	}
}

func TestParsePriority(t *testing.T) {
	tests := []struct {
		in      string
		want    task.Priority
		wantErr bool
	}{
		{in: "low", want: task.PriorityLow},
		{in: "Medium", want: task.PriorityMedium},
		{in: "HIGH", want: task.PriorityHigh},
		{in: "urgent", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := task.ParsePriority(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
			if !tt.wantErr && got.String() != strings.ToLower(tt.in) {
				t.Errorf("String() = %q, want %q", got.String(), strings.ToLower(tt.in))
			}
		})
	}
	if got := task.Priority(7).String(); got != "Priority(7)" {
		t.Errorf("String() = %q", got)
	}
}
