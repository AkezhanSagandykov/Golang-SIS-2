package storage_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"taskflow/internal/storage"
	"taskflow/internal/task"
)

var now = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

func newTask(t *testing.T, title string, tags ...string) task.Task {
	t.Helper()
	tk, err := task.New(task.Params{Title: title, Tags: tags}, now)
	if err != nil {
		t.Fatalf("task.New: %v", err)
	}
	return tk
}

func toQueued(tk *task.Task) error {
	return tk.TransitionTo(task.StatusQueued, now)
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
		t.Fatalf("Create changed caller's task: ID = %d", input.ID)
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
	updated, _ := s.Update(created.ID, func(tk *task.Task) error { return nil })
	updated.Tags[0] = "mutated-update"

	fresh, _ := s.Get(created.ID)
	if fresh.Title != "original" || fresh.Tags[0] != "go" {
		t.Fatalf("store state leaked: title=%q tags=%v", fresh.Title, fresh.Tags)
	}
}

func TestUpdate(t *testing.T) {
	s := storage.NewMemoryStore()
	created, _ := s.Create(newTask(t, "x"))

	updated, err := s.Update(created.ID, toQueued)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Status() != task.StatusQueued {
		t.Errorf("returned status = %s, want queued", updated.Status())
	}
	stored, _ := s.Get(created.ID)
	if stored.Status() != task.StatusQueued {
		t.Errorf("stored status = %s, want queued", stored.Status())
	}
}

func TestUpdate_ErrorLeavesTaskUnchanged(t *testing.T) {
	s := storage.NewMemoryStore()
	created, _ := s.Create(newTask(t, "x", "go"))
	errStop := errors.New("stop")

	_, err := s.Update(created.ID, func(tk *task.Task) error {
		tk.Title = "changed"
		tk.Tags[0] = "changed"
		return errStop
	})

	if !errors.Is(err, errStop) {
		t.Fatalf("err = %v, want errStop", err)
	}
	stored, _ := s.Get(created.ID)
	if stored.Title != "x" || stored.Tags[0] != "go" {
		t.Fatalf("failed Update changed the task: %+v", stored)
	}
}

func TestUpdate_CannotChangeID(t *testing.T) {
	s := storage.NewMemoryStore()
	created, _ := s.Create(newTask(t, "x"))

	updated, err := s.Update(created.ID, func(tk *task.Task) error {
		tk.ID = 42
		return nil
	})
	if err != nil || updated.ID != created.ID {
		t.Fatalf("Update = %+v, %v", updated, err)
	}
	if _, err := s.Get(42); !errors.Is(err, task.ErrNotFound) {
		t.Fatal("Update must not create a task under a new ID")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	s := storage.NewMemoryStore()
	called := false
	_, err := s.Update(42, func(tk *task.Task) error {
		called = true
		return nil
	})
	if !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if called || s.Len() != 0 {
		t.Fatal("Update of a missing task must not call change or create tasks")
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

func TestDelete_RefusesBusyTask(t *testing.T) {
	s := storage.NewMemoryStore()
	created, _ := s.Create(newTask(t, "x"))
	if _, err := s.Update(created.ID, toQueued); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(created.ID); !errors.Is(err, task.ErrTaskBusy) {
		t.Fatalf("Delete of queued task: err = %v, want ErrTaskBusy", err)
	}
	if s.Len() != 1 {
		t.Fatal("busy task was deleted")
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

func TestConcurrentCreateGivesUniqueIDs(t *testing.T) {
	s := storage.NewMemoryStore()
	const goroutines, perGoroutine = 20, 50

	var wg sync.WaitGroup
	ids := make(chan task.ID, goroutines*perGoroutine)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				created, err := s.Create(newTask(t, "x"))
				if err != nil {
					t.Errorf("Create: %v", err)
					return
				}
				ids <- created.ID
			}
		}()
	}
	wg.Wait()
	close(ids)

	seen := make(map[task.ID]bool)
	for id := range ids {
		if seen[id] {
			t.Fatalf("ID %d given twice", id)
		}
		seen[id] = true
	}
	if len(seen) != goroutines*perGoroutine || s.Len() != goroutines*perGoroutine {
		t.Fatalf("got %d unique IDs and %d tasks, want %d", len(seen), s.Len(), goroutines*perGoroutine)
	}
}

func TestUpdate_IsAtomic(t *testing.T) {
	s := storage.NewMemoryStore()
	created, _ := s.Create(newTask(t, "x"))
	const goroutines = 50

	start := make(chan struct{})
	results := make(chan error, goroutines)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.Update(created.ID, toQueued)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	succeeded := 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case !errors.Is(err, task.ErrInvalidTransition):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d goroutines moved the same task todo -> queued, want exactly 1", succeeded)
	}
}

func TestUpdate_NoLostUpdates(t *testing.T) {
	s := storage.NewMemoryStore()
	created, _ := s.Create(newTask(t, "x"))
	const goroutines = 100

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Update(created.ID, func(tk *task.Task) error {
				tk.Tags = append(tk.Tags, fmt.Sprint(g))
				return nil
			})
			if err != nil {
				t.Errorf("Update: %v", err)
			}
		}()
	}
	wg.Wait()

	stored, _ := s.Get(created.ID)
	if len(stored.Tags) != goroutines {
		t.Fatalf("len(Tags) = %d, want %d: some updates were lost", len(stored.Tags), goroutines)
	}
}

func TestConcurrentReadsAndWrites(t *testing.T) {
	s := storage.NewMemoryStore()
	for i := 0; i < 10; i++ {
		_, _ = s.Create(newTask(t, "x"))
	}

	var wg sync.WaitGroup
	for g := 0; g < 10; g++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_, _ = s.Create(newTask(t, "y"))
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				if _, err := s.Get(1); err != nil {
					t.Errorf("Get: %v", err)
					return
				}
				list, err := s.List()
				if err != nil || len(list) < 10 {
					t.Errorf("List: %d tasks, %v", len(list), err)
					return
				}
			}
		}()
	}
	wg.Wait()

	if s.Len() != 10+10*100 {
		t.Fatalf("Len = %d, want %d", s.Len(), 10+10*100)
	}
}
