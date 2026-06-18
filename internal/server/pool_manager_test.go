package server

import (
	"fmt"
	"sync"
	"testing"

	"lyric-video-factory/internal/render"
)

func makeClip(name string) render.Clip {
	return render.Clip{Path: "/clips/" + name, Duration: 10}
}

func TestPoolManager_Add(t *testing.T) {
	pm := &PoolManager{}
	pm.Add(makeClip("a.mp4"))
	pm.Add(makeClip("b.mp4"))
	if pm.Len() != 2 {
		t.Errorf("Len = %d, want 2", pm.Len())
	}
}

func TestPoolManager_AddDeduplicates(t *testing.T) {
	pm := &PoolManager{}
	pm.Add(makeClip("a.mp4"))
	pm.Add(makeClip("a.mp4"))
	if pm.Len() != 1 {
		t.Errorf("Len = %d, want 1 (duplicate should be ignored)", pm.Len())
	}
}

func TestPoolManager_Remove(t *testing.T) {
	pm := &PoolManager{}
	pm.Add(makeClip("a.mp4"))
	pm.Add(makeClip("b.mp4"))

	if ok := pm.Remove("a.mp4"); !ok {
		t.Error("Remove returned false for existing clip")
	}
	if pm.Len() != 1 {
		t.Errorf("Len = %d, want 1 after remove", pm.Len())
	}

	if ok := pm.Remove("nonexistent.mp4"); ok {
		t.Error("Remove returned true for nonexistent clip")
	}
}

func TestPoolManager_Clear(t *testing.T) {
	pm := &PoolManager{}
	pm.Add(makeClip("a.mp4"))
	pm.Add(makeClip("b.mp4"))

	if n := pm.Clear(); n != 2 {
		t.Errorf("Clear returned %d, want 2", n)
	}
	if pm.Len() != 0 {
		t.Errorf("Len = %d, want 0 after clear", pm.Len())
	}
}

func TestPoolManager_Entries_returnsCopy(t *testing.T) {
	pm := &PoolManager{}
	pm.Add(makeClip("a.mp4"))

	entries := pm.Entries()
	entries[0].Path = "mutated"

	original := pm.Entries()
	if original[0].Path == "mutated" {
		t.Error("Entries should return a copy; mutating it affected the pool")
	}
}

func TestPoolManager_AsPool(t *testing.T) {
	pm := &PoolManager{}
	pm.Add(makeClip("a.mp4"))
	pm.Add(makeClip("b.mp4"))

	pool := pm.AsPool()
	if len(pool.Clips) != 2 {
		t.Errorf("AsPool clips = %d, want 2", len(pool.Clips))
	}
}

func TestPoolManager_ConcurrentAdd(t *testing.T) {
	pm := &PoolManager{}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			pm.Add(render.Clip{Path: fmt.Sprintf("/clip%d.mp4", i), Duration: float64(i)})
		}()
	}
	wg.Wait()
	if pm.Len() != 100 {
		t.Errorf("Len = %d, want 100 after concurrent adds", pm.Len())
	}
}
