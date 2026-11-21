package worker

import (
	"errors"
	"fmt"
	"sync"

	"taskflow/internal/task"
)

const MinWorkers = 2

var (
	ErrInvalidConfig = errors.New("invalid worker pool config")
	ErrQueueFull     = errors.New("queue is full")
	ErrStopped       = errors.New("worker pool is stopped")
)

type Handler func(worker int, id task.ID)

type Pool struct {
	queue   chan task.ID
	handle  Handler
	workers int
	wg      sync.WaitGroup

	mu      sync.Mutex
	stopped bool
}

func NewPool(workers, queueSize int, handle Handler) (*Pool, error) {
	if workers < MinWorkers {
		return nil, fmt.Errorf("%w: need at least %d workers, got %d", ErrInvalidConfig, MinWorkers, workers)
	}
	if queueSize < 1 {
		return nil, fmt.Errorf("%w: queue size must be at least 1, got %d", ErrInvalidConfig, queueSize)
	}
	if handle == nil {
		return nil, fmt.Errorf("%w: handler is nil", ErrInvalidConfig)
	}

	p := &Pool{
		queue:   make(chan task.ID, queueSize),
		handle:  handle,
		workers: workers,
	}
	p.wg.Add(workers)
	for i := 1; i <= workers; i++ {
		go p.run(i)
	}
	return p, nil
}

func (p *Pool) run(worker int) {
	defer p.wg.Done()
	for id := range p.queue {
		p.handle(worker, id)
	}
}

func (p *Pool) Submit(id task.ID) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped {
		return ErrStopped
	}
	select {
	case p.queue <- id:
		return nil
	default:
		return ErrQueueFull
	}
}

func (p *Pool) Stop() {
	p.mu.Lock()
	if !p.stopped {
		p.stopped = true
		close(p.queue)
	}
	p.mu.Unlock()

	p.wg.Wait()
}

func (p *Pool) Workers() int {
	return p.workers
}

func (p *Pool) Capacity() int {
	return cap(p.queue)
}

func (p *Pool) Queued() int {
	return len(p.queue)
}
