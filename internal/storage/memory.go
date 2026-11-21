package storage

import (
	"cmp"
	"fmt"
	"slices"
	"sync"

	"taskflow/internal/task"
)

type MemoryStore struct {
	mu     sync.RWMutex
	tasks  map[task.ID]task.Task
	lastID task.ID
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tasks: make(map[task.ID]task.Task)}
}

func (s *MemoryStore) Create(t task.Task) (task.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tasks == nil {
		s.tasks = make(map[task.ID]task.Task)
	}
	s.lastID++
	t.ID = s.lastID
	s.tasks[t.ID] = t.Clone()
	return t.Clone(), nil
}

func (s *MemoryStore) Get(id task.ID) (task.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.tasks[id]
	if !ok {
		return task.Task{}, fmt.Errorf("task %d: %w", id, task.ErrNotFound)
	}
	return t.Clone(), nil
}

func (s *MemoryStore) Update(id task.ID, change func(t *task.Task) error) (task.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.tasks[id]
	if !ok {
		return task.Task{}, fmt.Errorf("task %d: %w", id, task.ErrNotFound)
	}
	t := stored.Clone()
	if err := change(&t); err != nil {
		return task.Task{}, err
	}
	t.ID = id
	s.tasks[id] = t.Clone()
	return t, nil
}

func (s *MemoryStore) Delete(id task.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %d: %w", id, task.ErrNotFound)
	}
	if t.Status().IsBusy() {
		return fmt.Errorf("task %d: %w", id, task.ErrTaskBusy)
	}
	delete(s.tasks, id)
	return nil
}

func (s *MemoryStore) List() ([]task.Task, error) {
	s.mu.RLock()
	result := make([]task.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		result = append(result, t.Clone())
	}
	s.mu.RUnlock()

	slices.SortFunc(result, func(a, b task.Task) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return result, nil
}

func (s *MemoryStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tasks)
}
