// Package job models a single transcription request as it moves through
// the pipeline (queued -> extracting audio -> transcribing -> done/failed/
// canceled) and holds the in-memory history of all jobs run this session.
package job

import (
	"sync"
	"time"
)

// Status is a job's place in its lifecycle.
type Status string

const (
	StatusQueued          Status = "queued"
	StatusExtractingAudio Status = "extracting_audio"
	StatusTranscribing    Status = "transcribing"
	StatusDone            Status = "done"
	StatusFailed          Status = "failed"
	StatusCanceled        Status = "canceled"
)

// IsTerminal reports whether a job in this status will never change state
// again (so e.g. it's no longer cancelable, and an SSE subscriber connecting
// after this point will never see another event for the job).
func (s Status) IsTerminal() bool {
	return s == StatusDone || s == StatusFailed || s == StatusCanceled
}

// OutputMode selects where the generated SRT is written.
type OutputMode string

const (
	OutputSameAsSource OutputMode = "same_as_source"
	OutputCustom       OutputMode = "custom"
)

// Request is the fully-validated input needed to run a transcription job.
// Paths are already absolute and cleaned by the time a Request is
// constructed (see httpapi's job handler).
type Request struct {
	MediaPath  string     `json:"mediaPath"`
	ModelID    string     `json:"modelId"`
	ModelPath  string     `json:"-"` // local filesystem detail, not part of the public API shape
	Language   string     `json:"language"`
	OutputMode OutputMode `json:"outputMode"`
	OutputDir  string     `json:"outputDir,omitempty"`  // only meaningful when OutputMode == OutputCustom
	MaxLen     int        `json:"maxLen,omitempty"`     // 0 = engine default
	MaxContext int        `json:"maxContext,omitempty"` // -mc passed to whisper-cli; 0 disables context carryover (default)
}

// Job is a Request plus its runtime state.
type Job struct {
	ID        string    `json:"id"`
	Request   Request   `json:"request"`
	Status    Status    `json:"status"`
	Percent   float64   `json:"percent"`
	Message   string    `json:"message,omitempty"`
	SRTPath   string    `json:"srtPath,omitempty"` // set once Status == StatusDone
	Error     string    `json:"error,omitempty"`   // set once Status == StatusFailed
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	cancel func() // set by the queue worker once the job starts running; unexported, never serialized
}

// Store is an in-memory, thread-safe collection of jobs, most-recent-first.
// History does not persist across restarts — an accepted MVP limitation
// (see the project plan's "known limitations" section).
type Store struct {
	mu   sync.RWMutex
	jobs []*Job // newest first
	byID map[string]*Job
}

func NewStore() *Store {
	return &Store{byID: make(map[string]*Job)}
}

func (s *Store) Add(j *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append([]*Job{j}, s.jobs...)
	s.byID[j.ID] = j
}

// Get returns a snapshot copy of the job (not the live pointer), so callers
// can read it freely without racing the worker goroutine's in-place updates.
func (s *Store) Get(id string) (Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.byID[id]
	if !ok {
		return Job{}, false
	}
	return *j, true
}

// List returns a snapshot copy of all jobs, newest first.
func (s *Store) List() []Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Job, len(s.jobs))
	for i, j := range s.jobs {
		out[i] = *j
	}
	return out
}

// Update mutates the job with the given ID under the store's lock and
// stamps UpdatedAt. Returns false if no such job exists.
func (s *Store) Update(id string, fn func(*Job)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.byID[id]
	if !ok {
		return false
	}
	fn(j)
	j.UpdatedAt = time.Now()
	return true
}

// SetCancel stores the cancel func the queue worker uses to interrupt this
// job's subprocess(es) when Cancel is requested.
func (s *Store) SetCancel(id string, cancel func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.byID[id]; ok {
		j.cancel = cancel
	}
}

// Cancel invokes the job's cancel func, if it has one (i.e. it has started
// running) and the job is still in a non-terminal status. Returns false if
// the job doesn't exist, hasn't started yet, or has already finished
// (done/failed/canceled) — status and cancel-func are read together under
// one lock so this can't race a concurrent Update marking the job terminal.
func (s *Store) Cancel(id string) bool {
	s.mu.RLock()
	j, ok := s.byID[id]
	var cancel func()
	if ok {
		if j.cancel != nil && !j.Status.IsTerminal() {
			cancel = j.cancel
		}
	}
	s.mu.RUnlock()

	if cancel == nil {
		return false
	}
	cancel()
	return true
}
