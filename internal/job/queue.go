package job

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"faster-whisper-gui/internal/transcribe"
)

// Pipeline holds everything the queue needs to actually run a job. Audio
// extraction and duration probing are injected as funcs (rather than
// exposing FfmpegPath/FfprobePath directly) so tests can exercise the
// queue's sequencing/cancellation logic with fakes instead of real
// subprocesses — see queue_test.go. internal/appserver wires these to
// transcribe.ExtractAudio/transcribe.ProbeDuration.
type Pipeline struct {
	ExtractAudio  func(ctx context.Context, mediaPath, outWavPath string) error
	ProbeDuration func(ctx context.Context, mediaPath string) (time.Duration, error)
	TmpDir        string
	Engine        transcribe.Engine
}

// EventPublisher receives per-job lifecycle/progress events. This decouples
// the queue from the SSE transport — internal/sseutil.Broker implements it structurally.
type EventPublisher interface {
	Publish(jobID string, eventType string, data any)
}

// Queue runs at most one transcription job at a time: a single GPU can't
// usefully run concurrent whisper-cli processes anyway, and it keeps job
// ordering predictable and easy to reason about.
type Queue struct {
	store    *Store
	pipeline Pipeline
	events   EventPublisher
	submit   chan *Job
}

func NewQueue(store *Store, pipeline Pipeline, events EventPublisher) *Queue {
	q := &Queue{
		store:    store,
		pipeline: pipeline,
		events:   events,
		submit:   make(chan *Job, 64),
	}
	go q.run()
	return q
}

// Submit creates a new job in Queued state and enqueues it for processing.
// The caller is expected to have already validated req (media file exists,
// output path is writable, model exists) — see httpapi's job handler.
func (q *Queue) Submit(req Request) (*Job, error) {
	id, err := newJobID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	j := &Job{
		ID:        id,
		Request:   req,
		Status:    StatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	q.store.Add(j)
	q.submit <- j
	return j, nil
}

func (q *Queue) run() {
	for j := range q.submit {
		q.process(j)
	}
}

func (q *Queue) process(j *Job) {
	ctx, cancel := context.WithCancel(context.Background())
	q.store.SetCancel(j.ID, cancel)
	defer cancel()

	tmpWav := filepath.Join(q.pipeline.TmpDir, j.ID+".wav")
	defer os.Remove(tmpWav)

	q.setStatus(j.ID, StatusExtractingAudio, 0, "Extracting audio")
	if err := q.pipeline.ExtractAudio(ctx, j.Request.MediaPath, tmpWav); err != nil {
		q.finishWithError(j.ID, ctx, err)
		return
	}

	duration, err := q.pipeline.ProbeDuration(ctx, j.Request.MediaPath)
	if err != nil {
		q.finishWithError(j.ID, ctx, err)
		return
	}

	q.setStatus(j.ID, StatusTranscribing, 0, "Transcribing")
	opts := transcribe.Options{
		AudioPath:     tmpWav,
		ModelPath:     j.Request.ModelPath,
		Language:      j.Request.Language,
		OutputPrefix:  OutputPrefix(j.Request),
		MaxLen:        j.Request.MaxLen,
		MaxContext:    j.Request.MaxContext,
		TotalDuration: duration,
	}
	err = q.pipeline.Engine.Transcribe(ctx, opts, func(p transcribe.Progress) {
		q.setStatus(j.ID, StatusTranscribing, p.Percent, p.Message)
	})
	if err != nil {
		q.finishWithError(j.ID, ctx, err)
		return
	}

	srtPath := opts.OutputPrefix + ".srt"
	q.store.Update(j.ID, func(job *Job) {
		job.Status = StatusDone
		job.Percent = 100
		job.SRTPath = srtPath
	})
	q.events.Publish(j.ID, "done", map[string]string{"srtPath": srtPath})
}

// finishWithError records a job as canceled (if its context was the reason
// the step failed) or failed (any other error).
func (q *Queue) finishWithError(id string, ctx context.Context, err error) {
	if ctx.Err() != nil {
		q.store.Update(id, func(j *Job) { j.Status = StatusCanceled })
		q.events.Publish(id, "canceled", map[string]string{})
		return
	}
	q.store.Update(id, func(j *Job) {
		j.Status = StatusFailed
		j.Error = err.Error()
	})
	q.events.Publish(id, "error", map[string]string{"message": err.Error()})
}

func (q *Queue) setStatus(id string, status Status, percent float64, message string) {
	q.store.Update(id, func(j *Job) {
		j.Status = status
		j.Percent = percent
		j.Message = message
	})
	q.events.Publish(id, "progress", map[string]any{
		"stage":   string(status),
		"percent": percent,
		"message": message,
	})
}

func newJobID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate job id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
