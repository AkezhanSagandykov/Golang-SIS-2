package service_test

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"taskflow/internal/service"
	"taskflow/internal/storage"
	"taskflow/internal/task"
	"taskflow/internal/worker"
)

var _ service.Repository = (*storage.MemoryStore)(nil)

const timeout = 2 * time.Second

var start = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type blockingProcessor struct {
	started chan task.ID
	release chan struct{}
	once    sync.Once
}

func (p *blockingProcessor) Process(t task.Task) (task.Result, error) {
	p.started <- t.ID
	<-p.release
	return task.Analyze(t.Description)
}

func (p *blockingProcessor) open() {
	p.once.Do(func() { close(p.release) })
}

type countingProcessor struct {
	mu    sync.Mutex
	calls map[task.ID]int
}

func (p *countingProcessor) Process(t task.Task) (task.Result, error) {
	p.mu.Lock()
	p.calls[t.ID]++
	p.mu.Unlock()
	return task.Analyze(t.Description)
}

func newService(t *testing.T, proc service.Processor, workers, queueSize int) (*service.Service, *fakeClock) {
	t.Helper()
	clock := &fakeClock{now: start}
	svc, err := service.New(storage.NewMemoryStore(), clock, proc, service.Config{Workers: workers, QueueSize: queueSize})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	t.Cleanup(svc.Shutdown)
	return svc, clock
}

func newBlockedService(t *testing.T, workers, queueSize int) (*service.Service, *blockingProcessor) {
	t.Helper()
	proc := &blockingProcessor{started: make(chan task.ID, 100), release: make(chan struct{})}
	svc, _ := newService(t, proc, workers, queueSize)
	t.Cleanup(proc.open)
	return svc, proc
}

func receive(t *testing.T, ch <-chan task.ID) task.ID {
	t.Helper()
	select {
	case id := <-ch:
		return id
	case <-time.After(timeout):
		t.Fatal("timed out waiting for a worker")
	}
	return 0
}

func mustCreate(t *testing.T, svc *service.Service, p task.Params) task.Task {
	t.Helper()
	tk, err := svc.Create(p)
	if err != nil {
		t.Fatalf("Create(%+v): %v", p, err)
	}
	return tk
}

func mustSubmit(t *testing.T, svc *service.Service, id task.ID) {
	t.Helper()
	if _, err := svc.Submit(id); err != nil {
		t.Fatalf("Submit(%d): %v", id, err)
	}
}

func mustGet(t *testing.T, svc *service.Service, id task.ID) task.Task {
	t.Helper()
	tk, err := svc.Get(id)
	if err != nil {
		t.Fatalf("Get(%d): %v", id, err)
	}
	return tk
}

func TestNew_InvalidConfig(t *testing.T) {
	for _, cfg := range []service.Config{{Workers: 1, QueueSize: 5}, {Workers: 2, QueueSize: 0}, {}} {
		svc, err := service.New(storage.NewMemoryStore(), nil, nil, cfg)
		if !errors.Is(err, worker.ErrInvalidConfig) || svc != nil {
			t.Errorf("New(%+v) = %v, %v; want ErrInvalidConfig", cfg, svc, err)
		}
	}
}

func TestCreateAndGet(t *testing.T) {
	svc, _ := newService(t, nil, 2, 5)
	tk := mustCreate(t, svc, task.Params{Title: "Сдать проект", Priority: task.PriorityHigh})

	if tk.ID != 1 || tk.Status() != task.StatusTodo || !tk.CreatedAt.Equal(start) {
		t.Fatalf("unexpected task: %+v", tk)
	}
	if got := mustGet(t, svc, tk.ID); got.Title != "Сдать проект" {
		t.Fatalf("Get = %+v", got)
	}
}

func TestCreate_ValidationErrorIsWrapped(t *testing.T) {
	svc, _ := newService(t, nil, 2, 5)
	_, err := svc.Create(task.Params{Title: "  "})

	if !errors.Is(err, task.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation in chain", err)
	}
	if err.Error() != "create: validation failed: title must not be empty" {
		t.Errorf("message = %q", err.Error())
	}
}

func TestSubmit_ProcessesTasksAndStoresTheirOwnResults(t *testing.T) {
	svc, _ := newService(t, nil, 3, 20)

	descriptions := make(map[task.ID]string)
	for i := 1; i <= 15; i++ {
		desc := fmt.Sprintf("задача%d %s", i, strings.Repeat("слово ", i))
		tk := mustCreate(t, svc, task.Params{Title: fmt.Sprint("t", i), Description: desc})
		descriptions[tk.ID] = desc
	}
	for id := range descriptions {
		queued, err := svc.Submit(id)
		if err != nil {
			t.Fatalf("Submit(%d): %v", id, err)
		}
		if queued.Status() != task.StatusQueued {
			t.Fatalf("Submit returned status %s, want queued", queued.Status())
		}
	}

	svc.Shutdown()

	for id, desc := range descriptions {
		got := mustGet(t, svc, id)
		want, _ := task.Analyze(strings.TrimSpace(desc))
		if got.Status() != task.StatusDone {
			t.Errorf("task %d: status %s, want done", id, got.Status())
			continue
		}
		if got.Result.Words != want.Words || got.Result.Characters != want.Characters ||
			!slices.Equal(got.Result.Keywords, want.Keywords) {
			t.Errorf("task %d: result %+v, want %+v", id, got.Result, want)
		}
		if got.Worker < 1 || got.Worker > 3 {
			t.Errorf("task %d: worker %d, want 1..3", id, got.Worker)
		}
	}
}

func TestSubmit_DoesNotWaitAndTasksRunConcurrently(t *testing.T) {
	svc, proc := newBlockedService(t, 2, 5)
	a := mustCreate(t, svc, task.Params{Title: "a", Description: "первая задача"})
	b := mustCreate(t, svc, task.Params{Title: "b", Description: "вторая задача"})

	mustSubmit(t, svc, a.ID)
	mustSubmit(t, svc, b.ID)

	first := receive(t, proc.started)
	second := receive(t, proc.started)
	if first == second {
		t.Fatalf("task %d started twice", first)
	}

	for _, id := range []task.ID{a.ID, b.ID} {
		if got := mustGet(t, svc, id); got.Status() != task.StatusInProgress {
			t.Errorf("task %d while both workers are busy: %s, want in_progress", id, got.Status())
		}
	}
	list, err := svc.List(service.Filter{Status: task.StatusInProgress})
	if err != nil || len(list) != 2 {
		t.Fatalf("List(in_progress) = %d tasks, %v; want 2", len(list), err)
	}

	proc.open()
	svc.Shutdown()

	for _, id := range []task.ID{a.ID, b.ID} {
		if got := mustGet(t, svc, id); got.Status() != task.StatusDone {
			t.Errorf("task %d after release: %s, want done", id, got.Status())
		}
	}
}

func TestSubmit_RejectsInvalidStates(t *testing.T) {
	svc, proc := newBlockedService(t, 2, 5)
	running := mustCreate(t, svc, task.Params{Title: "running", Description: "текст"})
	running2 := mustCreate(t, svc, task.Params{Title: "running2", Description: "текст"})
	waiting := mustCreate(t, svc, task.Params{Title: "waiting", Description: "текст"})
	cancelled := mustCreate(t, svc, task.Params{Title: "cancelled"})
	if _, err := svc.Cancel(cancelled.ID); err != nil {
		t.Fatal(err)
	}

	mustSubmit(t, svc, running.ID)
	mustSubmit(t, svc, running2.ID)
	receive(t, proc.started)
	receive(t, proc.started)
	mustSubmit(t, svc, waiting.ID)

	tests := []struct {
		name string
		id   task.ID
		want error
	}{
		{"in progress", running.ID, task.ErrInvalidTransition},
		{"queued", waiting.ID, task.ErrInvalidTransition},
		{"cancelled", cancelled.ID, task.ErrInvalidTransition},
		{"missing", 404, task.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.Submit(tt.id); !errors.Is(err, tt.want) {
				t.Fatalf("Submit: err = %v, want %v", err, tt.want)
			}
		})
	}

	if err := svc.Delete(running.ID); !errors.Is(err, task.ErrTaskBusy) {
		t.Errorf("Delete of running task: err = %v, want ErrTaskBusy", err)
	}
	if _, err := svc.Cancel(waiting.ID); !errors.Is(err, task.ErrInvalidTransition) {
		t.Errorf("Cancel of queued task: err = %v, want ErrInvalidTransition", err)
	}
	if _, err := svc.Reopen(waiting.ID); !errors.Is(err, task.ErrInvalidTransition) {
		t.Errorf("Reopen of queued task: err = %v, want ErrInvalidTransition", err)
	}

	proc.open()
	svc.Shutdown()

	if _, err := svc.Submit(running.ID); !errors.Is(err, task.ErrInvalidTransition) {
		t.Errorf("Submit of done task: err = %v, want ErrInvalidTransition", err)
	}
}

func TestSubmit_QueueFullKeepsTaskInTodo(t *testing.T) {
	svc, proc := newBlockedService(t, 2, 1)
	var ids []task.ID
	for i := 1; i <= 4; i++ {
		ids = append(ids, mustCreate(t, svc, task.Params{Title: fmt.Sprint("t", i), Description: "текст"}).ID)
	}

	mustSubmit(t, svc, ids[0])
	receive(t, proc.started)
	mustSubmit(t, svc, ids[1])
	receive(t, proc.started)
	mustSubmit(t, svc, ids[2])

	_, err := svc.Submit(ids[3])

	if !errors.Is(err, worker.ErrQueueFull) {
		t.Fatalf("Submit into a full queue: err = %v, want ErrQueueFull", err)
	}
	if got := mustGet(t, svc, ids[3]); got.Status() != task.StatusTodo {
		t.Fatalf("rejected task has status %s, want todo", got.Status())
	}
	if q := svc.Queue(); q.Queued != 1 || q.Capacity != 1 || q.Workers != 2 {
		t.Fatalf("Queue() = %+v", q)
	}

	proc.open()
	svc.Shutdown()

	for _, id := range ids[:3] {
		if got := mustGet(t, svc, id); got.Status() != task.StatusDone {
			t.Errorf("accepted task %d: %s, want done", id, got.Status())
		}
	}
	if got := mustGet(t, svc, ids[3]); got.Status() != task.StatusTodo {
		t.Errorf("rejected task: %s, want todo", got.Status())
	}
}

func TestSubmit_ProcessingFailureMarksTaskFailed(t *testing.T) {
	svc, _ := newService(t, nil, 2, 5)
	tk := mustCreate(t, svc, task.Params{Title: "без описания"})

	mustSubmit(t, svc, tk.ID)
	svc.Shutdown()

	got := mustGet(t, svc, tk.ID)
	if got.Status() != task.StatusFailed {
		t.Fatalf("status = %s, want failed", got.Status())
	}
	if got.FailReason != task.ErrNothingToAnalyze.Error() || got.Result.Words != 0 {
		t.Fatalf("FailReason = %q, Result = %+v", got.FailReason, got.Result)
	}
	if _, err := svc.Submit(tk.ID); !errors.Is(err, task.ErrInvalidTransition) {
		t.Errorf("Submit of failed task: err = %v, want ErrInvalidTransition", err)
	}
	if reopened, err := svc.Reopen(tk.ID); err != nil || reopened.Status() != task.StatusTodo {
		t.Errorf("Reopen = %s, %v; want todo", reopened.Status(), err)
	}
}

func TestSubmit_SameTaskFromManyGoroutines(t *testing.T) {
	proc := &countingProcessor{calls: make(map[task.ID]int)}
	svc, _ := newService(t, proc, 3, 10)
	tk := mustCreate(t, svc, task.Params{Title: "x", Description: "один текст"})

	const goroutines = 50
	var accepted atomic.Int32
	begin := make(chan struct{})
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-begin
			_, err := svc.Submit(tk.ID)
			switch {
			case err == nil:
				accepted.Add(1)
			case !errors.Is(err, task.ErrInvalidTransition):
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(begin)
	wg.Wait()
	svc.Shutdown()

	if accepted.Load() != 1 {
		t.Fatalf("%d submissions accepted, want exactly 1", accepted.Load())
	}
	proc.mu.Lock()
	calls := proc.calls[tk.ID]
	proc.mu.Unlock()
	if calls != 1 {
		t.Fatalf("task processed %d times, want 1", calls)
	}
	if got := mustGet(t, svc, tk.ID); got.Status() != task.StatusDone {
		t.Fatalf("status = %s, want done", got.Status())
	}
}

func TestConcurrentReadsDuringProcessing(t *testing.T) {
	svc, _ := newService(t, nil, 4, 50)
	for i := 1; i <= 40; i++ {
		mustCreate(t, svc, task.Params{Title: fmt.Sprint("t", i), Description: strings.Repeat("слово ", i)})
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for r := 0; r < 4; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				list, err := svc.List(service.Filter{})
				if err != nil || len(list) != 40 {
					t.Errorf("List = %d tasks, %v", len(list), err)
					return
				}
				for _, tk := range list {
					if tk.Status() == task.StatusDone && tk.Result.Words == 0 {
						t.Errorf("task %d is done but has no result", tk.ID)
						return
					}
				}
				if _, err := svc.Stats(); err != nil {
					t.Errorf("Stats: %v", err)
					return
				}
			}
		}()
	}

	for id := task.ID(1); id <= 40; id++ {
		mustSubmit(t, svc, id)
	}
	svc.Shutdown()
	close(stop)
	readers.Wait()

	stats, _ := svc.Stats()
	if stats[task.StatusDone] != 40 {
		t.Fatalf("done = %d, want 40 (stats %v)", stats[task.StatusDone], stats)
	}
}

func TestShutdown_FinishesAcceptedTasksAndRejectsNewOnes(t *testing.T) {
	svc, proc := newBlockedService(t, 2, 3)
	var accepted []task.ID
	for i := 1; i <= 5; i++ {
		accepted = append(accepted, mustCreate(t, svc, task.Params{Title: fmt.Sprint("t", i), Description: "текст"}).ID)
	}
	late := mustCreate(t, svc, task.Params{Title: "late", Description: "текст"})

	mustSubmit(t, svc, accepted[0])
	mustSubmit(t, svc, accepted[1])
	receive(t, proc.started)
	receive(t, proc.started)
	for _, id := range accepted[2:] {
		mustSubmit(t, svc, id)
	}

	stopped := make(chan struct{})
	go func() {
		svc.Shutdown()
		close(stopped)
	}()

	deadline := time.Now().Add(timeout)
	for {
		_, err := svc.Submit(late.ID)
		if errors.Is(err, worker.ErrStopped) {
			break
		}
		if !errors.Is(err, worker.ErrQueueFull) {
			t.Fatalf("Submit while shutting down: err = %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("service did not stop accepting tasks")
		}
		runtime.Gosched()
	}
	if got := mustGet(t, svc, late.ID); got.Status() != task.StatusTodo {
		t.Fatalf("task rejected after shutdown has status %s, want todo", got.Status())
	}

	select {
	case <-stopped:
		t.Fatal("Shutdown returned before accepted tasks finished")
	default:
	}

	proc.open()
	select {
	case <-stopped:
	case <-time.After(timeout):
		t.Fatal("Shutdown did not return")
	}

	for _, id := range accepted {
		if got := mustGet(t, svc, id); got.Status() != task.StatusDone {
			t.Errorf("task %d accepted before shutdown: %s, want done", id, got.Status())
		}
	}
	svc.Shutdown()
}

func TestCancelAndReopen(t *testing.T) {
	svc, _ := newService(t, nil, 2, 5)
	tk := mustCreate(t, svc, task.Params{Title: "x"})

	if got, err := svc.Cancel(tk.ID); err != nil || got.Status() != task.StatusCancelled {
		t.Fatalf("Cancel = %s, %v", got.Status(), err)
	}
	if got, err := svc.Reopen(tk.ID); err != nil || got.Status() != task.StatusTodo {
		t.Fatalf("Reopen = %s, %v", got.Status(), err)
	}
	_, err := svc.Reopen(tk.ID)
	if err == nil || err.Error() != "reopen: task 1: todo -> todo: invalid status transition" {
		t.Fatalf("Reopen of todo task: err = %v", err)
	}
}

func TestNotFound(t *testing.T) {
	svc, _ := newService(t, nil, 2, 5)
	calls := map[string]func() error{
		"get":    func() error { _, err := svc.Get(404); return err },
		"submit": func() error { _, err := svc.Submit(404); return err },
		"cancel": func() error { _, err := svc.Cancel(404); return err },
		"reopen": func() error { _, err := svc.Reopen(404); return err },
		"delete": func() error { return svc.Delete(404) },
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
	svc, _ := newService(t, nil, 2, 5)
	tk := mustCreate(t, svc, task.Params{Title: "x"})
	if err := svc.Delete(tk.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(tk.ID); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("Get after Delete: %v", err)
	}
}

func TestList_Filter(t *testing.T) {
	svc, _ := newService(t, nil, 2, 5)
	a := mustCreate(t, svc, task.Params{Title: "a", Priority: task.PriorityHigh, Tags: []string{"go"}})
	b := mustCreate(t, svc, task.Params{Title: "b", Priority: task.PriorityLow, Tags: []string{"go", "home"}})
	c := mustCreate(t, svc, task.Params{Title: "c", Priority: task.PriorityHigh})
	if _, err := svc.Cancel(b.ID); err != nil {
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
	svc, clock := newService(t, nil, 2, 5)
	later := mustCreate(t, svc, task.Params{Title: "later", Deadline: start.Add(5 * time.Hour)})
	late := mustCreate(t, svc, task.Params{Title: "late", Deadline: start.Add(2 * time.Hour)})
	done := mustCreate(t, svc, task.Params{Title: "done", Description: "текст", Deadline: start.Add(time.Hour)})
	mustCreate(t, svc, task.Params{Title: "no deadline"})
	mustCreate(t, svc, task.Params{Title: "future", Deadline: start.AddDate(0, 1, 0)})

	mustSubmit(t, svc, done.ID)
	svc.Shutdown()

	if got, _ := svc.Overdue(); len(got) != 0 {
		t.Fatalf("nothing should be overdue yet, got %v", idsOf(got))
	}

	clock.Advance(24 * time.Hour)
	got, err := svc.Overdue()
	if err != nil {
		t.Fatal(err)
	}
	if want := []task.ID{late.ID, later.ID}; !slices.Equal(idsOf(got), want) {
		t.Fatalf("Overdue = %v, want %v", idsOf(got), want)
	}
}

func TestStats(t *testing.T) {
	svc, _ := newService(t, nil, 2, 5)

	empty, err := svc.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != len(task.AllStatuses()) {
		t.Fatalf("Stats on empty store must contain every status, got %v", empty)
	}

	mustCreate(t, svc, task.Params{Title: "todo"})
	ok := mustCreate(t, svc, task.Params{Title: "ok", Description: "текст"})
	bad := mustCreate(t, svc, task.Params{Title: "bad"})
	cancelled := mustCreate(t, svc, task.Params{Title: "cancelled"})
	mustSubmit(t, svc, ok.ID)
	mustSubmit(t, svc, bad.ID)
	_, _ = svc.Cancel(cancelled.ID)
	svc.Shutdown()

	stats, _ := svc.Stats()
	want := map[task.Status]int{
		task.StatusTodo: 1, task.StatusQueued: 0, task.StatusInProgress: 0,
		task.StatusDone: 1, task.StatusFailed: 1, task.StatusCancelled: 1,
	}
	for status, n := range want {
		if stats[status] != n {
			t.Errorf("stats[%s] = %d, want %d", status, stats[status], n)
		}
	}
}

func TestNew_DefaultClockAndProcessor(t *testing.T) {
	svc, err := service.New(storage.NewMemoryStore(), nil, nil, service.Config{Workers: 2, QueueSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Shutdown)

	before := time.Now()
	tk := mustCreate(t, svc, task.Params{Title: "x", Description: "Hello Go world"})
	if tk.CreatedAt.Before(before) || tk.CreatedAt.After(time.Now()) {
		t.Fatalf("CreatedAt %v is not the current time", tk.CreatedAt)
	}
	mustSubmit(t, svc, tk.ID)
	svc.Shutdown()
	if got := mustGet(t, svc, tk.ID); got.Result.Words != 3 {
		t.Fatalf("default processor result = %+v, want 3 words", got.Result)
	}
}

type brokenRepo struct {
	service.Repository
	err error
}

func (r brokenRepo) Create(task.Task) (task.Task, error) { return task.Task{}, r.err }
func (r brokenRepo) Get(task.ID) (task.Task, error)      { return task.Task{}, r.err }
func (r brokenRepo) Update(task.ID, func(*task.Task) error) (task.Task, error) {
	return task.Task{}, r.err
}
func (r brokenRepo) Delete(task.ID) error       { return r.err }
func (r brokenRepo) List() ([]task.Task, error) { return nil, r.err }

var errDB = errors.New("database is down")

func TestRepositoryErrorsArePropagated(t *testing.T) {
	svc, err := service.New(brokenRepo{err: errDB}, &fakeClock{now: start}, nil, service.Config{Workers: 2, QueueSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Shutdown)

	calls := map[string]func() error{
		"create":  func() error { _, err := svc.Create(task.Params{Title: "x"}); return err },
		"get":     func() error { _, err := svc.Get(1); return err },
		"submit":  func() error { _, err := svc.Submit(1); return err },
		"cancel":  func() error { _, err := svc.Cancel(1); return err },
		"reopen":  func() error { _, err := svc.Reopen(1); return err },
		"delete":  func() error { return svc.Delete(1) },
		"list":    func() error { _, err := svc.List(service.Filter{}); return err },
		"overdue": func() error { _, err := svc.Overdue(); return err },
		"stats":   func() error { _, err := svc.Stats(); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, errDB) {
				t.Fatalf("err = %v, want errDB in chain", err)
			}
		})
	}
}

func idsOf(tasks []task.Task) []task.ID {
	var ids []task.ID
	for _, tk := range tasks {
		ids = append(ids, tk.ID)
	}
	return ids
}
