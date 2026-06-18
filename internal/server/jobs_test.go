package server

import (
	"sync"
	"testing"
)

func TestJobStore_CreateAndGet(t *testing.T) {
	store := newJobStore()
	job := store.create("test.json", nil, 3, 0.5)

	if job.ID == "" {
		t.Error("job ID should not be empty")
	}
	if job.Status != StatusPending {
		t.Errorf("status = %q, want pending", job.Status)
	}
	if job.N != 3 {
		t.Errorf("N = %d, want 3", job.N)
	}

	got, ok := store.get(job.ID)
	if !ok {
		t.Fatal("job not found after create")
	}
	if got.ID != job.ID {
		t.Error("retrieved wrong job")
	}
}

func TestJobStore_GetUnknownID(t *testing.T) {
	store := newJobStore()
	if _, ok := store.get("nonexistent"); ok {
		t.Error("expected false for unknown ID")
	}
}

func TestJob_StateTransitions_Done(t *testing.T) {
	store := newJobStore()
	job := store.create("t.json", nil, 1, 0)

	if job.Snapshot().Status != StatusPending {
		t.Error("initial status should be pending")
	}

	job.setRunning()
	if job.Snapshot().Status != StatusRunning {
		t.Error("status should be running after setRunning")
	}

	job.setDone([]string{"variant_01.mp4"})
	snap := job.Snapshot()
	if snap.Status != StatusDone {
		t.Errorf("status = %q, want done", snap.Status)
	}
	if len(snap.Results) != 1 || snap.Results[0] != "variant_01.mp4" {
		t.Errorf("results = %v", snap.Results)
	}
}

func TestJob_StateTransitions_Error(t *testing.T) {
	store := newJobStore()
	job := store.create("t.json", nil, 1, 0)
	job.setError("something went wrong")

	snap := job.Snapshot()
	if snap.Status != StatusError {
		t.Errorf("status = %q, want error", snap.Status)
	}
	if snap.Error != "something went wrong" {
		t.Errorf("error message = %q", snap.Error)
	}
}

func TestJob_Snapshot_isIndependent(t *testing.T) {
	store := newJobStore()
	job := store.create("t.json", nil, 1, 0)
	job.setDone([]string{"file.mp4"})

	snap := job.Snapshot()
	snap.Results[0] = "mutated"

	snap2 := job.Snapshot()
	if snap2.Results[0] != "file.mp4" {
		t.Error("mutating snapshot Results affected the original job")
	}
}

func TestJobStore_List(t *testing.T) {
	store := newJobStore()
	store.create("a.json", nil, 1, 0)
	store.create("b.json", nil, 2, 0)

	views := store.list()
	if len(views) != 2 {
		t.Errorf("list len = %d, want 2", len(views))
	}
}

func TestJobStore_ConcurrentAccess(t *testing.T) {
	store := newJobStore()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			store.create("t.json", nil, 1, 0)
		}()
		go func() {
			defer wg.Done()
			store.list()
		}()
	}
	wg.Wait()
}

func TestNewID_uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := newID()
		if seen[id] {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		seen[id] = true
	}
}
