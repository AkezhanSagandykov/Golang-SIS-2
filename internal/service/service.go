package service

import (
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"taskflow/internal/task"
	"taskflow/internal/worker"
)

const (
	DefaultWorkers   = 3
	DefaultQueueSize = 10
)

type Repository interface {
	Create(t task.Task) (task.Task, error)
	Get(id task.ID) (task.Task, error)
	Update(id task.ID, change func(t *task.Task) error) (task.Task, error)
	Delete(id task.ID) error
	List() ([]task.Task, error)
}

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

type Processor interface {
	Process(t task.Task) (task.Result, error)
}

type DescriptionAnalyzer struct{}

func (DescriptionAnalyzer) Process(t task.Task) (task.Result, error) {
	return task.Analyze(t.Description)
}

type Config struct {
	Workers   int
	QueueSize int
}

type QueueInfo struct {
	Workers  int
	Queued   int
	Capacity int
}

type Service struct {
	repo  Repository
	clock Clock
	proc  Processor
	pool  *worker.Pool
}

func New(repo Repository, clock Clock, proc Processor, cfg Config) (*Service, error) {
	if clock == nil {
		clock = SystemClock{}
	}
	if proc == nil {
		proc = DescriptionAnalyzer{}
	}
	s := &Service{repo: repo, clock: clock, proc: proc}

	pool, err := worker.NewPool(cfg.Workers, cfg.QueueSize, s.process)
	if err != nil {
		return nil, fmt.Errorf("new service: %w", err)
	}
	s.pool = pool
	return s, nil
}

func (s *Service) Create(p task.Params) (task.Task, error) {
	t, err := task.New(p, s.clock.Now())
	if err != nil {
		return task.Task{}, fmt.Errorf("create: %w", err)
	}
	created, err := s.repo.Create(t)
	if err != nil {
		return task.Task{}, fmt.Errorf("create: %w", err)
	}
	return created, nil
}

func (s *Service) Get(id task.ID) (task.Task, error) {
	t, err := s.repo.Get(id)
	if err != nil {
		return task.Task{}, fmt.Errorf("get: %w", err)
	}
	return t, nil
}

func (s *Service) Submit(id task.ID) (task.Task, error) {
	queued, err := s.repo.Update(id, func(t *task.Task) error {
		return t.TransitionTo(task.StatusQueued, s.clock.Now())
	})
	if err != nil {
		return task.Task{}, fmt.Errorf("submit: %w", err)
	}

	if err := s.pool.Submit(id); err != nil {
		_, undoErr := s.repo.Update(id, func(t *task.Task) error {
			return t.TransitionTo(task.StatusTodo, s.clock.Now())
		})
		if undoErr != nil {
			return task.Task{}, fmt.Errorf("submit: task %d: %w; undo: %w", id, err, undoErr)
		}
		return task.Task{}, fmt.Errorf("submit: task %d: %w", id, err)
	}
	return queued, nil
}

func (s *Service) process(workerID int, id task.ID) {
	t, err := s.repo.Update(id, func(t *task.Task) error {
		return t.Start(workerID, s.clock.Now())
	})
	if err != nil {
		log.Printf("worker %d: skip task %d: %v", workerID, id, err)
		return
	}

	result, procErr := s.proc.Process(t)

	_, err = s.repo.Update(id, func(t *task.Task) error {
		if procErr != nil {
			return t.Fail(procErr.Error(), s.clock.Now())
		}
		return t.Complete(result, s.clock.Now())
	})
	if err != nil {
		log.Printf("worker %d: save task %d: %v", workerID, id, err)
	}
}

func (s *Service) Shutdown() {
	s.pool.Stop()
}

func (s *Service) Queue() QueueInfo {
	return QueueInfo{
		Workers:  s.pool.Workers(),
		Queued:   s.pool.Queued(),
		Capacity: s.pool.Capacity(),
	}
}

func (s *Service) Cancel(id task.ID) (task.Task, error) {
	t, err := s.repo.Update(id, func(t *task.Task) error {
		return t.TransitionTo(task.StatusCancelled, s.clock.Now())
	})
	if err != nil {
		return task.Task{}, fmt.Errorf("cancel: %w", err)
	}
	return t, nil
}

func (s *Service) Reopen(id task.ID) (task.Task, error) {
	t, err := s.repo.Update(id, func(t *task.Task) error {
		return t.Reopen(s.clock.Now())
	})
	if err != nil {
		return task.Task{}, fmt.Errorf("reopen: %w", err)
	}
	return t, nil
}

func (s *Service) Delete(id task.ID) error {
	if err := s.repo.Delete(id); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}

type Filter struct {
	Status   task.Status
	Priority task.Priority
	Tag      string
}

func (f Filter) Matches(t task.Task) bool {
	if f.Status != 0 && t.Status() != f.Status {
		return false
	}
	if f.Priority != 0 && t.Priority != f.Priority {
		return false
	}
	if strings.TrimSpace(f.Tag) != "" && !t.HasTag(f.Tag) {
		return false
	}
	return true
}

func (s *Service) List(f Filter) ([]task.Task, error) {
	all, err := s.repo.List()
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	var result []task.Task
	for _, t := range all {
		if f.Matches(t) {
			result = append(result, t)
		}
	}
	return result, nil
}

func (s *Service) Overdue() ([]task.Task, error) {
	all, err := s.repo.List()
	if err != nil {
		return nil, fmt.Errorf("overdue: %w", err)
	}
	now := s.clock.Now()
	var result []task.Task
	for _, t := range all {
		if t.IsOverdue(now) {
			result = append(result, t)
		}
	}
	slices.SortFunc(result, func(a, b task.Task) int {
		return a.Deadline.Compare(b.Deadline)
	})
	return result, nil
}

func (s *Service) Stats() (map[task.Status]int, error) {
	all, err := s.repo.List()
	if err != nil {
		return nil, fmt.Errorf("stats: %w", err)
	}
	statuses := task.AllStatuses()
	counts := make(map[task.Status]int, len(statuses))
	for _, st := range statuses {
		counts[st] = 0
	}
	for _, t := range all {
		counts[t.Status()]++
	}
	return counts, nil
}
