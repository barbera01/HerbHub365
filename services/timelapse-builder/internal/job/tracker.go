// Package job provides an in-memory tracker for timelapse build jobs.
package job

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Params mirrors the user-supplied build options.
type Params struct {
	From          string  `json:"from,omitempty"`
	To            string  `json:"to,omitempty"`
	OutputName    string  `json:"output_name,omitempty"`
	InputFPS      int     `json:"input_fps"`
	OutputFPS     int     `json:"output_fps"`
	CRF           int     `json:"crf"`
	MinBrightness float64 `json:"min_brightness"`
}

type Job struct {
	ID          string    `json:"id"`
	Status      Status    `json:"status"`
	Params      Params    `json:"params"`
	OutputFile  string    `json:"output_file,omitempty"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Log         string    `json:"log,omitempty"`
}

type Tracker struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

func NewTracker() *Tracker {
	return &Tracker{jobs: make(map[string]*Job)}
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (t *Tracker) Create(p Params) *Job {
	j := &Job{
		ID:        newID(),
		Status:    StatusQueued,
		Params:    p,
		CreatedAt: time.Now().UTC(),
	}
	t.mu.Lock()
	t.jobs[j.ID] = j
	t.mu.Unlock()
	return j
}

func (t *Tracker) Get(id string) (*Job, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	j, ok := t.jobs[id]
	return j, ok
}

func (t *Tracker) All() []*Job {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]*Job, 0, len(t.jobs))
	for _, j := range t.jobs {
		out = append(out, j)
	}
	return out
}

func (t *Tracker) MarkRunning(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[id]; ok {
		j.Status = StatusRunning
		j.StartedAt = time.Now().UTC()
	}
}

func (t *Tracker) MarkCompleted(id, outputFile, log string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[id]; ok {
		j.Status = StatusCompleted
		j.OutputFile = outputFile
		j.Log = log
		j.CompletedAt = time.Now().UTC()
	}
}

func (t *Tracker) MarkFailed(id, errMsg, log string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[id]; ok {
		j.Status = StatusFailed
		j.Error = errMsg
		j.Log = log
		j.CompletedAt = time.Now().UTC()
	}
}
