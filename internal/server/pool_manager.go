package server

import (
	"path/filepath"
	"sync"

	"lyric-video-factory/internal/render"
)

// PoolManager хранит рабочий список клипов в памяти.
// Пул не привязан к содержимому директорий — пользователь сам решает, что в нём есть.
// Физического удаления файлов не происходит никогда.
type PoolManager struct {
	mu    sync.RWMutex
	clips []render.Clip
}

// newPoolManager создаёт менеджер и инициализирует пул из переданных директорий.
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

// unsafeAdd добавляет клип без блокировки (вызывать под pm.mu.Lock).
func (pm *PoolManager) unsafeAdd(c render.Clip) {
	name := filepath.Base(c.Path)
	for _, existing := range pm.clips {
		if filepath.Base(existing.Path) == name {
			return
		}
	}
	pm.clips = append(pm.clips, c)
}

// Add добавляет клип в пул (потокобезопасно, дубликаты по имени пропускаются).
func (pm *PoolManager) Add(c render.Clip) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.unsafeAdd(c)
}

// Remove удаляет клип из пула по имени файла. Возвращает true если клип найден.
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

// Clear удаляет все клипы из пула. Возвращает количество удалённых.
func (pm *PoolManager) Clear() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	n := len(pm.clips)
	pm.clips = nil
	return n
}

// Entries возвращает копию текущего списка клипов.
func (pm *PoolManager) Entries() []render.Clip {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	out := make([]render.Clip, len(pm.clips))
	copy(out, pm.clips)
	return out
}

// AsPool возвращает *render.Pool для использования при рендеринге.
func (pm *PoolManager) AsPool() *render.Pool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	clips := make([]render.Clip, len(pm.clips))
	copy(clips, pm.clips)
	return &render.Pool{Clips: clips}
}

// Len возвращает количество клипов в пуле.
func (pm *PoolManager) Len() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.clips)
}
