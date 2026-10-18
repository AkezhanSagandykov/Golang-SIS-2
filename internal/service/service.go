package service

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"taskflow/internal/task"
)

type Repository interface {
	Create(t task.Task) (task.Task, error)
	Get(id task.ID) (task.Task, error)
	Update(t task.Task) error
	Delete(id task.ID) error
	List() ([]task.Task, error)
}

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

type Service struct {
	repo  Repository
	clock Clock
}

func New(repo Repository, clock Clock) *Service {
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{repo: repo, clock: clock}
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

func (s *Service) Start(id task.ID) (task.Task, error) {
	return s.changeStatus("start", id, task.StatusInProgress)
}

func (s *Service) Complete(id task.ID) (task.Task, error) {
	return s.changeStatus("complete", id, task.StatusDone)
}

func (s *Service) Cancel(id task.ID) (task.Task, error) {
	return s.changeStatus("cancel", id, task.StatusCancelled)
}

func (s *Service) Reopen(id task.ID) (task.Task, error) {
	return s.changeStatus("reopen", id, task.StatusTodo)
}

func (s *Service) changeStatus(op string, id task.ID, to task.Status) (task.Task, error) {
	t, err := s.repo.Get(id)
	if err != nil {
		return task.Task{}, fmt.Errorf("%s: %w", op, err)
	}
	if err := t.TransitionTo(to, s.clock.Now()); err != nil {
		return task.Task{}, fmt.Errorf("%s: %w", op, err)
	}
	if err := s.repo.Update(t); err != nil {
		return task.Task{}, fmt.Errorf("%s: %w", op, err)
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
