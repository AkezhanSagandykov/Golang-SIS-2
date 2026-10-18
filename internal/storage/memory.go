package storage

import (
	"cmp"
	"fmt"
	"slices"

	"taskflow/internal/task"
)

type MemoryStore struct {
	tasks  map[task.ID]task.Task
	lastID task.ID
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tasks: make(map[task.ID]task.Task)}
}

func (s *MemoryStore) Create(t task.Task) (task.Task, error) {
	if s.tasks == nil {
		s.tasks = make(map[task.ID]task.Task)
	}
	s.lastID++
	t.ID = s.lastID
	s.tasks[t.ID] = t.Clone()
	return t.Clone(), nil
}

func (s *MemoryStore) Get(id task.ID) (task.Task, error) {
	t, ok := s.tasks[id]
	if !ok {
		return task.Task{}, fmt.Errorf("task %d: %w", id, task.ErrNotFound)
	}
	return t.Clone(), nil
}

func (s *MemoryStore) Update(t task.Task) error {
	if _, ok := s.tasks[t.ID]; !ok {
		return fmt.Errorf("task %d: %w", t.ID, task.ErrNotFound)
	}
	s.tasks[t.ID] = t.Clone()
	return nil
}

func (s *MemoryStore) Delete(id task.ID) error {
	if _, ok := s.tasks[id]; !ok {
		return fmt.Errorf("task %d: %w", id, task.ErrNotFound)
	}
	delete(s.tasks, id)
	return nil
}

func (s *MemoryStore) List() ([]task.Task, error) {
	result := make([]task.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		result = append(result, t.Clone())
	}
	slices.SortFunc(result, func(a, b task.Task) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return result, nil
}

func (s *MemoryStore) Len() int {
	return len(s.tasks)
}
