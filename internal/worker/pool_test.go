package worker_test

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"taskflow/internal/task"
	"taskflow/internal/worker"
)

const timeout = 2 * time.Second

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(timeout):
		t.Fatal("timed out waiting for a worker")
	}
	var zero T
	return zero
}

func newPool(t *testing.T, workers, queueSize int, h worker.Handler) *worker.Pool {
	t.Helper()
	p, err := worker.NewPool(workers, queueSize, h)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	t.Cleanup(p.Stop)
	return p
}

type recorder struct {
	mu      sync.Mutex
	handled map[task.ID]int
	workers map[int]bool
}

func newRecorder() *recorder {
	return &recorder{handled: make(map[task.ID]int), workers: make(map[int]bool)}
}

func (r *recorder) handle(worker int, id task.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handled[id]++
	r.workers[worker] = true
}

func (r *recorder) count(id task.ID) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.handled[id]
}

type gate struct {
	started chan task.ID
	release chan struct{}
	once    sync.Once
	done    *recorder
}

func newGatedPool(t *testing.T, workers, queueSize int) (*worker.Pool, *gate) {
	t.Helper()
	g := &gate{
		started: make(chan task.ID, 100),
		release: make(chan struct{}),
		done:    newRecorder(),
	}
	p := newPool(t, workers, queueSize, g.handle)
	t.Cleanup(g.open)
	return p, g
}

func (g *gate) handle(worker int, id task.ID) {
	g.started <- id
	<-g.release
	g.done.handle(worker, id)
}

func (g *gate) open() {
	g.once.Do(func() { close(g.release) })
}

func TestNewPool_InvalidConfig(t *testing.T) {
	noop := func(int, task.ID) {}
	tests := []struct {
		name      string
		workers   int
		queueSize int
		handler   worker.Handler
	}{
		{"one worker", 1, 5, noop},
		{"zero workers", 0, 5, noop},
		{"zero queue", 2, 0, noop},
		{"nil handler", 2, 5, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := worker.NewPool(tt.workers, tt.queueSize, tt.handler)
			if !errors.Is(err, worker.ErrInvalidConfig) || p != nil {
				t.Fatalf("NewPool = %v, %v; want nil, ErrInvalidConfig", p, err)
			}
		})
	}
}

func TestPool_StartsAndProcessesAllTasks(t *testing.T) {
	rec := newRecorder()
	p := newPool(t, 3, 20, rec.handle)

	if p.Workers() != 3 || p.Capacity() != 20 || p.Queued() != 0 {
		t.Fatalf("new pool: workers=%d capacity=%d queued=%d", p.Workers(), p.Capacity(), p.Queued())
	}
	for id := task.ID(1); id <= 20; id++ {
		if err := p.Submit(id); err != nil {
			t.Fatalf("Submit(%d): %v", id, err)
		}
	}
	p.Stop()

	for id := task.ID(1); id <= 20; id++ {
		if n := rec.count(id); n != 1 {
			t.Errorf("task %d handled %d times, want 1", id, n)
		}
	}
	for w := range rec.workers {
		if w < 1 || w > 3 {
			t.Errorf("unexpected worker number %d", w)
		}
	}
}

func TestPool_ProcessesTasksConcurrently(t *testing.T) {
	p, g := newGatedPool(t, 2, 5)

	_ = p.Submit(1)
	_ = p.Submit(2)

	first := receive(t, g.started)
	second := receive(t, g.started)

	if first == second {
		t.Fatalf("the same task started twice: %d", first)
	}
	if g.done.count(1)+g.done.count(2) != 0 {
		t.Fatal("a task finished before release")
	}
	g.open()
	p.Stop()

	if g.done.count(1) != 1 || g.done.count(2) != 1 {
		t.Fatalf("handled: %v", g.done.handled)
	}
}

func TestPool_RespectsWorkerLimit(t *testing.T) {
	var active, maxActive atomic.Int32
	started := make(chan task.ID, 10)
	release := make(chan struct{})
	var once sync.Once
	open := func() { once.Do(func() { close(release) }) }
	p := newPool(t, 3, 10, func(_ int, id task.ID) {
		n := active.Add(1)
		for {
			m := maxActive.Load()
			if n <= m || maxActive.CompareAndSwap(m, n) {
				break
			}
		}
		started <- id
		<-release
		active.Add(-1)
	})
	t.Cleanup(open)

	for id := task.ID(1); id <= 6; id++ {
		if err := p.Submit(id); err != nil {
			t.Fatalf("Submit(%d): %v", id, err)
		}
	}
	for i := 0; i < 3; i++ {
		receive(t, started)
	}

	if got := p.Queued(); got != 3 {
		t.Errorf("Queued = %d while all 3 workers are busy, want 3", got)
	}
	open()
	p.Stop()

	if got := maxActive.Load(); got != 3 {
		t.Fatalf("max tasks processed at the same time = %d, want 3", got)
	}
	if len(started) != 3 {
		t.Fatalf("remaining tasks started: %d, want 3", len(started))
	}
}

func TestPool_RejectsWhenQueueFull(t *testing.T) {
	p, g := newGatedPool(t, 2, 2)

	_ = p.Submit(1)
	_ = p.Submit(2)
	receive(t, g.started)
	receive(t, g.started)

	if err := p.Submit(3); err != nil {
		t.Fatalf("Submit(3): %v", err)
	}
	if err := p.Submit(4); err != nil {
		t.Fatalf("Submit(4): %v", err)
	}
	if err := p.Submit(5); !errors.Is(err, worker.ErrQueueFull) {
		t.Fatalf("Submit(5) into a full queue: err = %v, want ErrQueueFull", err)
	}
	if p.Queued() != 2 {
		t.Fatalf("Queued = %d, want 2 (the capacity)", p.Queued())
	}

	g.open()
	p.Stop()

	for id := task.ID(1); id <= 4; id++ {
		if g.done.count(id) != 1 {
			t.Errorf("accepted task %d handled %d times, want 1", id, g.done.count(id))
		}
	}
	if g.done.count(5) != 0 {
		t.Error("rejected task 5 was handled")
	}
}

func TestPool_StopFinishesAcceptedTasks(t *testing.T) {
	p, g := newGatedPool(t, 2, 2)

	_ = p.Submit(1)
	_ = p.Submit(2)
	receive(t, g.started)
	receive(t, g.started)
	_ = p.Submit(3)
	_ = p.Submit(4)
	if p.Queued() != 2 {
		t.Fatalf("Queued = %d, want 2", p.Queued())
	}

	stopped := make(chan struct{})
	go func() {
		p.Stop()
		close(stopped)
	}()

	deadline := time.Now().Add(timeout)
	for {
		err := p.Submit(99)
		if errors.Is(err, worker.ErrStopped) {
			break
		}
		if !errors.Is(err, worker.ErrQueueFull) {
			t.Fatalf("Submit while stopping: err = %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("pool did not stop accepting tasks")
		}
		runtime.Gosched()
	}

	select {
	case <-stopped:
		t.Fatal("Stop returned while tasks were still running")
	default:
	}

	g.open()
	receive(t, stopped)

	for id := task.ID(1); id <= 4; id++ {
		if g.done.count(id) != 1 {
			t.Errorf("task %d accepted before Stop was handled %d times, want 1", id, g.done.count(id))
		}
	}
	if g.done.count(99) != 0 {
		t.Error("task submitted after Stop was handled")
	}
}

func TestPool_SubmitAfterStopAndDoubleStop(t *testing.T) {
	rec := newRecorder()
	p := newPool(t, 2, 5, rec.handle)

	p.Stop()
	if err := p.Submit(1); !errors.Is(err, worker.ErrStopped) {
		t.Fatalf("Submit after Stop: err = %v, want ErrStopped", err)
	}
	p.Stop()

	if rec.count(1) != 0 {
		t.Fatal("task submitted after Stop was handled")
	}
}

func TestPool_ConcurrentSubmitAndStop(t *testing.T) {
	rec := newRecorder()
	p := newPool(t, 4, 8, rec.handle)

	const submitters, perSubmitter = 10, 100
	var accepted atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for s := 0; s < submitters; s++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < perSubmitter; i++ {
				id := task.ID(s*perSubmitter + i + 1)
				err := p.Submit(id)
				switch {
				case err == nil:
					accepted.Add(1)
				case errors.Is(err, worker.ErrQueueFull), errors.Is(err, worker.ErrStopped):
				default:
					t.Errorf("Submit(%d): unexpected error %v", id, err)
				}
			}
		}()
	}

	close(start)
	go p.Stop()
	wg.Wait()
	p.Stop()

	handled := 0
	rec.mu.Lock()
	for id, n := range rec.handled {
		if n != 1 {
			t.Errorf("task %d handled %d times", id, n)
		}
		handled += n
	}
	rec.mu.Unlock()

	if handled != int(accepted.Load()) {
		t.Fatalf("accepted %d tasks but handled %d: tasks were lost", accepted.Load(), handled)
	}
}

func TestPool_WorkersExitAfterStop(t *testing.T) {
	before := runtime.NumGoroutine()
	p, err := worker.NewPool(5, 5, func(int, task.ID) {})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.NumGoroutine() < before+5 {
		t.Fatalf("expected 5 worker goroutines to start")
	}

	p.Stop()

	deadline := time.Now().Add(timeout)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("goroutines: before=%d after Stop=%d, workers leaked", before, runtime.NumGoroutine())
		}
		runtime.Gosched()
	}
}
