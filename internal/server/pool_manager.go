package server

import (
	"path/filepath"
	"sync"

	"lyric-video-factory/internal/render"
)

// PoolManager holds the working clip list in memory.
// The pool is not tied to directory contents — the user decides what's in it.
// Files are never physically deleted.
type PoolManager struct {
	mu    sync.RWMutex
	clips []render.Clip
}

// newPoolManager creates a manager and seeds the pool from the given directories.
func newPoolManager(dirs ...string) *PoolManager {
	pm := &PoolManager{}
	for _, dir := range dirs {
		pool, err := render.LoadPool(dir)
		if err != nil {
			continue
		}
		for _, c := range pool.Clips {
			pm.unsafeAdd(c)
		}
	}
	return pm
}

// unsafeAdd adds a clip without locking (must be called with pm.mu held).
func (pm *PoolManager) unsafeAdd(c render.Clip) {
	name := filepath.Base(c.Path)
	for _, existing := range pm.clips {
		if filepath.Base(existing.Path) == name {
			return
		}
	}
	pm.clips = append(pm.clips, c)
}

// Add adds a clip to the pool (thread-safe; duplicates by filename are ignored).
func (pm *PoolManager) Add(c render.Clip) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.unsafeAdd(c)
}

// Remove deletes a clip from the pool by filename. Returns true if the clip was found.
func (pm *PoolManager) Remove(name string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for i, c := range pm.clips {
		if filepath.Base(c.Path) == name {
			pm.clips = append(pm.clips[:i], pm.clips[i+1:]...)
			return true
		}
	}
	return false
}

// Clear removes all clips from the pool. Returns the number of clips removed.
func (pm *PoolManager) Clear() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	n := len(pm.clips)
	pm.clips = nil
	return n
}

// Entries returns a copy of the current clip list.
func (pm *PoolManager) Entries() []render.Clip {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	out := make([]render.Clip, len(pm.clips))
	copy(out, pm.clips)
	return out
}

// AsPool returns a *render.Pool ready for use during rendering.
func (pm *PoolManager) AsPool() *render.Pool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	clips := make([]render.Clip, len(pm.clips))
	copy(clips, pm.clips)
	return &render.Pool{Clips: clips}
}

// Len returns the number of clips in the pool.
func (pm *PoolManager) Len() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.clips)
}
