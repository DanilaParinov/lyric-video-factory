package server

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"lyric-video-factory/internal/tmpl"
)

type JobStatus string

const (
	StatusPending JobStatus = "pending"
	StatusRunning JobStatus = "running"
	StatusDone    JobStatus = "done"
	StatusError   JobStatus = "error"
)

// Job хранит состояние одной задачи генерации.
// Мутабельные поля защищены mu; читать через Snapshot().
type Job struct {
	mu           sync.RWMutex
	ID           string
	Status       JobStatus
	Error        string
	N            int
	Template     string
	TemplateData *tmpl.Template
	DimLevel     float64
	CreatedAt    time.Time
	Results      []string
}

func (j *Job) Snapshot() JobView {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return JobView{
		ID:        j.ID,
		Status:    j.Status,
		Error:     j.Error,
		N:         j.N,
		Template:  j.Template,
		DimLevel:  j.DimLevel,
		CreatedAt: j.CreatedAt,
		Results:   append([]string(nil), j.Results...),
	}
}

func (j *Job) setRunning() {
	j.mu.Lock()
	j.Status = StatusRunning
	j.mu.Unlock()
}

func (j *Job) setDone(results []string) {
	j.mu.Lock()
	j.Status = StatusDone
	j.Results = results
	j.mu.Unlock()
}

func (j *Job) setError(msg string) {
	j.mu.Lock()
	j.Status = StatusError
	j.Error = msg
	j.mu.Unlock()
}

// JobView — сериализуемый снимок задачи для JSON-ответов.
type JobView struct {
	ID        string    `json:"id"`
	Status    JobStatus `json:"status"`
	Error     string    `json:"error,omitempty"`
	N         int       `json:"n"`
	Template  string    `json:"template"`
	DimLevel  float64   `json:"dim_level"`
	CreatedAt time.Time `json:"created_at"`
	Results   []string  `json:"results,omitempty"`
}

// JobStore — потокобезопасное хранилище задач в памяти.
type JobStore struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

func newJobStore() *JobStore {
	return &JobStore{jobs: make(map[string]*Job)}
}

func (s *JobStore) create(templatePath string, templateData *tmpl.Template, n int, dimLevel float64) *Job {
	job := &Job{
		ID:           newID(),
		Status:       StatusPending,
		N:            n,
		Template:     templatePath,
		TemplateData: templateData,
		DimLevel:     dimLevel,
		CreatedAt:    time.Now(),
	}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()
	return job
}

func (s *JobStore) get(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	return j, ok
}

func (s *JobStore) list() []JobView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	views := make([]JobView, 0, len(s.jobs))
	for _, j := range s.jobs {
		views = append(views, j.Snapshot())
	}
	return views
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
