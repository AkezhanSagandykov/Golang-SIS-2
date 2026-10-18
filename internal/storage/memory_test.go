package storage_test

import (
	"errors"
	"testing"
	"time"

	"taskflow/internal/storage"
	"taskflow/internal/task"
)

var now = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

func newTask(t *testing.T, title string, tags ...string) task.Task {
	t.Helper()
	tk, err := task.New(task.Params{Title: title, Tags: tags}, now)
	if err != nil {
		t.Fatalf("task.New: %v", err)
	}
	return tk
}

func TestCreate_AssignsSequentialIDs(t *testing.T) {
	s := storage.NewMemoryStore()
	for want := task.ID(1); want <= 3; want++ {
		created, err := s.Create(newTask(t, "x"))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if created.ID != want {
			t.Errorf("ID = %d, want %d", created.ID, want)
		}
	}
	if s.Len() != 3 {
		t.Errorf("Len = %d, want 3", s.Len())
	}
}

func TestCreate_IDsAreNotReusedAfterDelete(t *testing.T) {
	s := storage.NewMemoryStore()
	first, _ := s.Create(newTask(t, "a"))
	if err := s.Delete(first.ID); err != nil {
		t.Fatal(err)
	}
	second, _ := s.Create(newTask(t, "b"))
	if second.ID == first.ID {
		t.Fatalf("ID %d reused after delete", second.ID)
	}
}

func TestCreate_DoesNotChangeCallersTask(t *testing.T) {
	s := storage.NewMemoryStore()
	input := newTask(t, "x")
	_, _ = s.Create(input)
	if input.ID != 0 {
		t.Fatalf("Create changed caller's task: ID = %d (task is passed by value)", input.ID)
	}
}

func TestZeroValueStoreIsUsable(t *testing.T) {
	var s storage.MemoryStore

	if _, err := s.Get(1); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("Get on empty store: err = %v", err)
	}
	created, err := s.Create(newTask(t, "x"))
	if err != nil || created.ID != 1 {
		t.Fatalf("Create on zero store: %+v, %v", created, err)
	}
}

func TestGet(t *testing.T) {
	s := storage.NewMemoryStore()
	created, _ := s.Create(newTask(t, "hello"))

	got, err := s.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "hello" || got.ID != created.ID {
		t.Errorf("Get = %+v", got)
	}

	_, err = s.Get(999)
	if !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("Get(999): err = %v, want ErrNotFound", err)
	}
	if err.Error() != "task 999: task not found" {
		t.Errorf("error message = %q", err.Error())
	}
}

func TestStore_ReturnsIndependentCopies(t *testing.T) {
	s := storage.NewMemoryStore()
	input := newTask(t, "original", "go")
	created, _ := s.Create(input)

	input.Tags[0] = "mutated-input"
	created.Title = "mutated-created"
	created.Tags[0] = "mutated-created"
	got, _ := s.Get(created.ID)
	got.Tags[0] = "mutated-get"
	list, _ := s.List()
	list[0].Tags[0] = "mutated-list"

	fresh, _ := s.Get(created.ID)
	if fresh.Title != "original" || fresh.Tags[0] != "go" {
		t.Fatalf("store state leaked: title=%q tags=%v", fresh.Title, fresh.Tags)
	}
}

func TestUpdate(t *testing.T) {
	s := storage.NewMemoryStore()
	created, _ := s.Create(newTask(t, "x"))

	if err := created.TransitionTo(task.StatusInProgress, now); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Get(created.ID)
	if before.Status() != task.StatusTodo {
		t.Fatalf("store changed without Update: %s", before.Status())
	}

	if err := s.Update(created); err != nil {
		t.Fatalf("Update: %v", err)
	}
	after, _ := s.Get(created.ID)
	if after.Status() != task.StatusInProgress {
		t.Errorf("status after Update = %s, want in_progress", after.Status())
	}
}

func TestUpdate_NotFound(t *testing.T) {
	s := storage.NewMemoryStore()
	ghost := newTask(t, "ghost")
	ghost.ID = 42
	if err := s.Update(ghost); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if s.Len() != 0 {
		t.Fatal("Update must not create tasks")
	}
}

func TestDelete(t *testing.T) {
	s := storage.NewMemoryStore()
	created, _ := s.Create(newTask(t, "x"))

	if err := s.Delete(created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(created.ID); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("Get after Delete: err = %v", err)
	}
	if err := s.Delete(created.ID); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("second Delete: err = %v, want ErrNotFound", err)
	}
}

func TestList_SortedByID(t *testing.T) {
	s := storage.NewMemoryStore()
	for i := 0; i < 50; i++ {
		_, _ = s.Create(newTask(t, "x"))
	}
	_ = s.Delete(10)
	_ = s.Delete(25)

	for run := 0; run < 5; run++ {
		list, err := s.List()
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 48 {
			t.Fatalf("len = %d, want 48", len(list))
		}
		for i := 1; i < len(list); i++ {
			if list[i-1].ID >= list[i].ID {
				t.Fatalf("run %d: not sorted at %d: %d >= %d", run, i, list[i-1].ID, list[i].ID)
			}
		}
	}
}

func TestList_Empty(t *testing.T) {
	list, err := storage.NewMemoryStore().List()
	if err != nil || len(list) != 0 {
		t.Fatalf("List on empty store = %v, %v", list, err)
	}
}
