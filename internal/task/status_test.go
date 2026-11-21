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
		{task.StatusTodo, task.StatusQueued}:       true,
		{task.StatusTodo, task.StatusCancelled}:    true,
		{task.StatusQueued, task.StatusInProgress}: true,
		{task.StatusQueued, task.StatusTodo}:       true,
		{task.StatusInProgress, task.StatusDone}:   true,
		{task.StatusInProgress, task.StatusFailed}: true,
		{task.StatusFailed, task.StatusTodo}:       true,
		{task.StatusCancelled, task.StatusTodo}:    true,
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

func TestStatus_IsClosedAndIsBusy(t *testing.T) {
	tests := []struct {
		status     task.Status
		wantClosed bool
		wantBusy   bool
	}{
		{task.StatusTodo, false, false},
		{task.StatusQueued, false, true},
		{task.StatusInProgress, false, true},
		{task.StatusDone, true, false},
		{task.StatusFailed, false, false},
		{task.StatusCancelled, true, false},
	}
	for _, tt := range tests {
		if got := tt.status.IsClosed(); got != tt.wantClosed {
			t.Errorf("%s.IsClosed() = %v, want %v", tt.status, got, tt.wantClosed)
		}
		if got := tt.status.IsBusy(); got != tt.wantBusy {
			t.Errorf("%s.IsBusy() = %v, want %v", tt.status, got, tt.wantBusy)
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
		{in: "QUEUED", want: task.StatusQueued},
		{in: "In_Progress", want: task.StatusInProgress},
		{in: "  done ", want: task.StatusDone},
		{in: "failed", want: task.StatusFailed},
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
