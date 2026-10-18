package service_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"taskflow/internal/service"
	"taskflow/internal/storage"
	"taskflow/internal/task"
)

var _ service.Repository = (*storage.MemoryStore)(nil)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

var start = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

func newService() (*service.Service, *fakeClock) {
	clock := &fakeClock{now: start}
	return service.New(storage.NewMemoryStore(), clock), clock
}

func mustCreate(t *testing.T, svc *service.Service, p task.Params) task.Task {
	t.Helper()
	tk, err := svc.Create(p)
	if err != nil {
		t.Fatalf("Create(%+v): %v", p, err)
	}
	return tk
}

func TestCreate(t *testing.T) {
	svc, _ := newService()
	tk := mustCreate(t, svc, task.Params{Title: "Сдать проект", Priority: task.PriorityHigh})

	if tk.ID != 1 || tk.Status() != task.StatusTodo || !tk.CreatedAt.Equal(start) {
		t.Fatalf("unexpected task: %+v", tk)
	}
	got, err := svc.Get(tk.ID)
	if err != nil || got.Title != "Сдать проект" {
		t.Fatalf("Get = %+v, %v", got, err)
	}
}

func TestCreate_ValidationErrorIsWrapped(t *testing.T) {
	svc, _ := newService()
	_, err := svc.Create(task.Params{Title: "  "})

	if !errors.Is(err, task.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation in chain", err)
	}
	if err.Error() != "create: validation failed: title must not be empty" {
		t.Errorf("message = %q", err.Error())
	}
}

func TestStatusChanges(t *testing.T) {
	svc, clock := newService()
	tk := mustCreate(t, svc, task.Params{Title: "x"})

	steps := []struct {
		name string
		do   func(task.ID) (task.Task, error)
		want task.Status
	}{
		{"start", svc.Start, task.StatusInProgress},
		{"reopen", svc.Reopen, task.StatusTodo},
		{"start again", svc.Start, task.StatusInProgress},
		{"complete", svc.Complete, task.StatusDone},
	}
	for _, step := range steps {
		clock.Advance(time.Hour)
		got, err := step.do(tk.ID)
		if err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		if got.Status() != step.want || !got.UpdatedAt.Equal(clock.Now()) {
			t.Fatalf("%s: status=%s updated=%v", step.name, got.Status(), got.UpdatedAt)
		}
		stored, _ := svc.Get(tk.ID)
		if stored.Status() != step.want {
			t.Fatalf("%s: change was not saved, stored status = %s", step.name, stored.Status())
		}
	}
}

func TestCancelAndReopen(t *testing.T) {
	svc, _ := newService()
	tk := mustCreate(t, svc, task.Params{Title: "x"})

	if got, err := svc.Cancel(tk.ID); err != nil || got.Status() != task.StatusCancelled {
		t.Fatalf("Cancel = %s, %v", got.Status(), err)
	}
	if got, err := svc.Reopen(tk.ID); err != nil || got.Status() != task.StatusTodo {
		t.Fatalf("Reopen = %s, %v", got.Status(), err)
	}
}

func TestInvalidTransition_IsNotSaved(t *testing.T) {
	svc, clock := newService()
	tk := mustCreate(t, svc, task.Params{Title: "x"})
	clock.Advance(time.Hour)

	_, err := svc.Complete(tk.ID)

	var te *task.TransitionError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want *TransitionError in chain", err)
	}
	if te.From != task.StatusTodo || te.To != task.StatusDone {
		t.Errorf("TransitionError = %+v", *te)
	}
	if err.Error() != "complete: task 1: todo -> done: invalid status transition" {
		t.Errorf("message = %q", err.Error())
	}
	stored, _ := svc.Get(tk.ID)
	if stored.Status() != task.StatusTodo || !stored.UpdatedAt.Equal(start) {
		t.Fatalf("failed transition changed stored task: %s, %v", stored.Status(), stored.UpdatedAt)
	}
}

func TestNotFound(t *testing.T) {
	svc, _ := newService()
	calls := map[string]func() error{
		"get":      func() error { _, err := svc.Get(404); return err },
		"start":    func() error { _, err := svc.Start(404); return err },
		"complete": func() error { _, err := svc.Complete(404); return err },
		"cancel":   func() error { _, err := svc.Cancel(404); return err },
		"reopen":   func() error { _, err := svc.Reopen(404); return err },
		"delete":   func() error { return svc.Delete(404) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, task.ErrNotFound) {
				t.Fatalf("err = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	svc, _ := newService()
	tk := mustCreate(t, svc, task.Params{Title: "x"})
	if err := svc.Delete(tk.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(tk.ID); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("Get after Delete: %v", err)
	}
}

func TestList_Filter(t *testing.T) {
	svc, _ := newService()
	a := mustCreate(t, svc, task.Params{Title: "a", Priority: task.PriorityHigh, Tags: []string{"go"}})
	b := mustCreate(t, svc, task.Params{Title: "b", Priority: task.PriorityLow, Tags: []string{"go", "home"}})
	c := mustCreate(t, svc, task.Params{Title: "c", Priority: task.PriorityHigh})
	if _, err := svc.Start(b.ID); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		filter service.Filter
		want   []task.ID
	}{
		{"empty filter = all", service.Filter{}, []task.ID{a.ID, b.ID, c.ID}},
		{"by status", service.Filter{Status: task.StatusTodo}, []task.ID{a.ID, c.ID}},
		{"by priority", service.Filter{Priority: task.PriorityHigh}, []task.ID{a.ID, c.ID}},
		{"by tag", service.Filter{Tag: "#GO"}, []task.ID{a.ID, b.ID}},
		{"combined", service.Filter{Status: task.StatusTodo, Tag: "go"}, []task.ID{a.ID}},
		{"nothing matches", service.Filter{Status: task.StatusDone}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.List(tt.filter)
			if err != nil {
				t.Fatal(err)
			}
			if ids := idsOf(got); !slices.Equal(ids, tt.want) {
				t.Errorf("got %v, want %v", ids, tt.want)
			}
		})
	}
}

func TestOverdue(t *testing.T) {
	svc, clock := newService()
	later := mustCreate(t, svc, task.Params{Title: "later", Deadline: start.Add(5 * time.Hour)})
	late := mustCreate(t, svc, task.Params{Title: "late", Deadline: start.Add(2 * time.Hour)})
	done := mustCreate(t, svc, task.Params{Title: "done", Deadline: start.Add(time.Hour)})
	mustCreate(t, svc, task.Params{Title: "no deadline"})
	mustCreate(t, svc, task.Params{Title: "future", Deadline: start.AddDate(0, 1, 0)})

	if _, err := svc.Start(done.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Complete(done.ID); err != nil {
		t.Fatal(err)
	}

	if got, _ := svc.Overdue(); len(got) != 0 {
		t.Fatalf("nothing should be overdue yet, got %v", idsOf(got))
	}

	clock.Advance(24 * time.Hour)
	got, err := svc.Overdue()
	if err != nil {
		t.Fatal(err)
	}
	if want := []task.ID{late.ID, later.ID}; !slices.Equal(idsOf(got), want) {
		t.Fatalf("Overdue = %v, want %v (done and no-deadline excluded, sorted by deadline)", idsOf(got), want)
	}
}

func TestStats(t *testing.T) {
	svc, _ := newService()

	empty, err := svc.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != len(task.AllStatuses()) {
		t.Fatalf("Stats on empty store must still contain every status, got %v", empty)
	}

	for i := 0; i < 3; i++ {
		mustCreate(t, svc, task.Params{Title: "x"})
	}
	_, _ = svc.Start(1)
	_, _ = svc.Start(2)
	_, _ = svc.Complete(2)

	stats, _ := svc.Stats()
	want := map[task.Status]int{
		task.StatusTodo: 1, task.StatusInProgress: 1, task.StatusDone: 1, task.StatusCancelled: 0,
	}
	for status, n := range want {
		if stats[status] != n {
			t.Errorf("stats[%s] = %d, want %d", status, stats[status], n)
		}
	}
}

func TestNew_DefaultClock(t *testing.T) {
	svc := service.New(storage.NewMemoryStore(), nil)
	before := time.Now()
	tk := mustCreate(t, svc, task.Params{Title: "x"})
	if tk.CreatedAt.Before(before) || tk.CreatedAt.After(time.Now()) {
		t.Fatalf("CreatedAt %v is not the current time", tk.CreatedAt)
	}
}

type brokenRepo struct {
	service.Repository
	err error
}

func (r brokenRepo) Create(task.Task) (task.Task, error) { return task.Task{}, r.err }
func (r brokenRepo) Get(task.ID) (task.Task, error)      { return task.Task{}, r.err }
func (r brokenRepo) Delete(task.ID) error                { return r.err }
func (r brokenRepo) List() ([]task.Task, error)          { return nil, r.err }

type updateFailsRepo struct {
	*storage.MemoryStore
	err error
}

func (r updateFailsRepo) Update(task.Task) error { return r.err }

var errDB = errors.New("database is down")

func TestRepositoryErrorsArePropagated(t *testing.T) {
	svc := service.New(brokenRepo{err: errDB}, &fakeClock{now: start})

	calls := map[string]func() error{
		"create":  func() error { _, err := svc.Create(task.Params{Title: "x"}); return err },
		"get":     func() error { _, err := svc.Get(1); return err },
		"start":   func() error { _, err := svc.Start(1); return err },
		"delete":  func() error { return svc.Delete(1) },
		"list":    func() error { _, err := svc.List(service.Filter{}); return err },
		"overdue": func() error { _, err := svc.Overdue(); return err },
		"stats":   func() error { _, err := svc.Stats(); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if !errors.Is(err, errDB) {
				t.Fatalf("err = %v, want errDB in chain", err)
			}
		})
	}
}

func TestUpdateErrorIsPropagated(t *testing.T) {
	store := storage.NewMemoryStore()
	repo := updateFailsRepo{MemoryStore: store, err: errDB}
	svc := service.New(repo, &fakeClock{now: start})

	tk := mustCreate(t, svc, task.Params{Title: "x"})
	_, err := svc.Start(tk.ID)
	if !errors.Is(err, errDB) {
		t.Fatalf("err = %v, want errDB", err)
	}
	if err.Error() != "start: database is down" {
		t.Errorf("message = %q", err.Error())
	}
}

func idsOf(tasks []task.Task) []task.ID {
	var ids []task.ID
	for _, tk := range tasks {
		ids = append(ids, tk.ID)
	}
	return ids
}
