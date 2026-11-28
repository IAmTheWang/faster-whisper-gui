package job

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	// PostProcess, if set, runs on the finished .srt (given its path) before
	// the job is marked done — internal/appserver wires it to the repetition
	// cleanup in internal/subtitle. A failure is logged but never fails the
	// job: the subtitle file whisper-cli wrote is still valid without it.
	PostProcess func(srtPath string) error
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

	// submitMu makes "no unfinished job targets this .srt" + "add the job" one
	// atomic step, so two concurrent submissions can't both pass the check.
	submitMu sync.Mutex
	closed   bool // set by Shutdown (under submitMu); Submit then refuses new jobs

	workerDone chan struct{} // closed when the worker goroutine has exited

	// controllers holds the pause/resume handle of each job whose engine
	// process is running; pauseMu also serializes Pause/Resume so the status
	// flip and the process freeze/thaw can't interleave.
	pauseMu     sync.Mutex
	controllers map[string]transcribe.Controller
	held        bool          // queue-wide pause: no job starts, a running one is frozen
	resumeCh    chan struct{} // closed (and replaced) when the hold is lifted

	stopCh chan struct{} // closed by Shutdown, releases a worker waiting on the hold
}

func NewQueue(store *Store, pipeline Pipeline, events EventPublisher) *Queue {
	q := &Queue{
		store:    store,
		pipeline: pipeline,
		events:   events,
		submit:   make(chan *Job, 64),

		workerDone:  make(chan struct{}),
		controllers: make(map[string]transcribe.Controller),
		resumeCh:    make(chan struct{}),
		stopCh:      make(chan struct{}),
	}
	go q.run()
	return q
}

// progressInterval throttles per-job progress updates.
const progressInterval = 200 * time.Millisecond

// ErrQueueFull is returned by Submit when too many jobs are already waiting.
var ErrQueueFull = errors.New("job queue is full")

// ErrQueueClosed is returned by Submit once Shutdown has been called.
var ErrQueueClosed = errors.New("job queue is shutting down")

// ErrDuplicateTarget is returned by Submit when another unfinished job already
// writes the same .srt file; the second job would silently clobber the first.
var ErrDuplicateTarget = errors.New("another unfinished job is already writing this subtitle file")

// Submit creates a new job in Queued state and enqueues it for processing.
// The caller is expected to have already validated req (media file exists,
// output path is writable, model exists) — see httpapi's job handler.
func (q *Queue) Submit(req Request) (*Job, error) {
	id, err := newJobID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	// The context is created here, not when the worker picks the job up, so a
	// job that is still waiting in the queue can be canceled too.
	ctx, cancel := context.WithCancel(context.Background())
	j := &Job{
		ID:        id,
		Request:   req,
		Status:    StatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		ctx:       ctx,
		cancel:    cancel,
	}
	q.submitMu.Lock()
	defer q.submitMu.Unlock()
	if q.closed {
		cancel()
		return nil, ErrQueueClosed
	}
	srtPath := OutputPrefix(req) + ".srt"
	for _, other := range q.store.List() {
		if !other.Status.IsTerminal() && strings.EqualFold(OutputPrefix(other.Request)+".srt", srtPath) {
			cancel()
			return nil, ErrDuplicateTarget
		}
	}
	q.store.Add(j)
	select {
	case q.submit <- j:
		return j, nil
	default:
		// Don't block the HTTP handler behind a full queue.
		q.store.Remove(id)
		cancel()
		return nil, ErrQueueFull
	}
}

// Cancel requests cancellation of a queued or running job. A job that has not
// started yet is marked canceled immediately (the worker would otherwise only
// notice once it reached it); a running job is canceled via its context and
// ends up canceled once its subprocess unwinds. Returns false if the job
// doesn't exist or has already finished.
func (q *Queue) Cancel(id string) bool {
	if !q.store.Cancel(id) {
		return false
	}
	canceled := false
	q.store.Update(id, func(j *Job) {
		if j.Status == StatusQueued {
			j.Status = StatusCanceled
			canceled = true
		}
	})
	if canceled {
		q.events.Publish(id, "canceled", map[string]string{})
	}
	return true
}

// ErrNotRunning is returned by Pause when the job has no running transcription
// to freeze (not found, still queued or extracting audio, or finished).
var ErrNotRunning = errors.New("job is not transcribing")

// ErrNotPaused is returned by Resume for a job that is not paused.
var ErrNotPaused = errors.New("job is not paused")

// Pause freezes a transcribing job's whisper-cli process: no CPU/GPU work is
// done until Resume, and progress picks up exactly where it stopped. The job
// keeps occupying the single worker, so queued jobs wait behind it. Cancel
// still works on a paused job.
func (q *Queue) Pause(id string) error {
	q.pauseMu.Lock()
	defer q.pauseMu.Unlock()
	ctl := q.controllers[id]
	cur, ok := q.store.Get(id)
	if ctl == nil || !ok || cur.Status != StatusTranscribing {
		return ErrNotRunning
	}
	if err := ctl.Suspend(); err != nil {
		if errors.Is(err, transcribe.ErrProcessGone) {
			return ErrNotRunning // it finished while we were looking
		}
		return fmt.Errorf("pause: %w", err)
	}
	q.setPaused(id, true)
	return nil
}

// Resume thaws a job frozen by Pause.
func (q *Queue) Resume(id string) error {
	q.pauseMu.Lock()
	defer q.pauseMu.Unlock()
	ctl := q.controllers[id]
	cur, ok := q.store.Get(id)
	if !ok || cur.Status != StatusPaused {
		return ErrNotPaused
	}
	if ctl == nil {
		return ErrNotPaused
	}
	if err := ctl.Resume(); err != nil {
		if errors.Is(err, transcribe.ErrProcessGone) {
			return ErrNotPaused
		}
		return fmt.Errorf("resume: %w", err)
	}
	q.setPaused(id, false)
	return nil
}

// setPaused flips a job between transcribing and paused (never touching a job
// that reached a terminal state meanwhile) and publishes the change.
func (q *Queue) setPaused(id string, paused bool) {
	to, from := StatusTranscribing, StatusPaused
	message := "Transcribing"
	if paused {
		to, from = StatusPaused, StatusTranscribing
		message = "Paused"
	}
	var percent float64
	changed := false
	q.store.Update(id, func(j *Job) {
		if j.Status != from {
			return
		}
		changed = true
		j.Status = to
		j.Message = message
		percent = j.Percent
	})
	if changed {
		q.events.Publish(id, "progress", map[string]any{
			"stage": string(to), "percent": percent, "message": message,
		})
	}
}

func (q *Queue) setController(id string, ctl transcribe.Controller) {
	q.pauseMu.Lock()
	if ctl == nil {
		delete(q.controllers, id)
		q.pauseMu.Unlock()
		return
	}
	q.controllers[id] = ctl
	// The queue was paused while this job was still extracting audio: freeze it
	// the moment whisper-cli exists.
	// The status flip happens before the lock is released, so no Pause/Resume/
	// ResumeAll can observe the frozen process with a "transcribing" status.
	if q.held {
		if err := ctl.Suspend(); err != nil {
			if !errors.Is(err, transcribe.ErrProcessGone) {
				slog.Warn("could not freeze job on a paused queue", "job", id, "error", err)
				cur, _ := q.store.Get(id)
				q.setStatus(id, StatusTranscribing, cur.Percent, "Queue is paused, but this job could not be frozen and keeps running")
			}
		} else {
			q.setPaused(id, true)
		}
	}
	q.pauseMu.Unlock()
}

// Held reports whether the whole queue is paused.
func (q *Queue) Held() bool {
	q.pauseMu.Lock()
	defer q.pauseMu.Unlock()
	return q.held
}

// PauseAll holds the queue: no further job starts, and the running one (if it
// is transcribing) is frozen. A job still extracting audio freezes as soon as
// transcription begins. Idempotent.
func (q *Queue) PauseAll() error {
	q.pauseMu.Lock()
	defer q.pauseMu.Unlock()
	q.held = true
	var errs []error
	for id, ctl := range q.controllers {
		if cur, ok := q.store.Get(id); ok && cur.Status == StatusTranscribing {
			if err := ctl.Suspend(); err != nil {
				if !errors.Is(err, transcribe.ErrProcessGone) { // already finishing: nothing to freeze
					errs = append(errs, err)
				}
				continue
			}
			q.setPaused(id, true)
		}
	}
	return errors.Join(errs...)
}

// ResumeAll lifts the hold and thaws every paused job. Idempotent.
func (q *Queue) ResumeAll() error {
	q.pauseMu.Lock()
	defer q.pauseMu.Unlock()
	if q.held {
		q.held = false
		close(q.resumeCh)
		q.resumeCh = make(chan struct{})
	}
	var errs []error
	for id, ctl := range q.controllers {
		if cur, ok := q.store.Get(id); ok && cur.Status == StatusPaused {
			if err := ctl.Resume(); err != nil {
				if !errors.Is(err, transcribe.ErrProcessGone) {
					errs = append(errs, err)
				}
				continue
			}
			q.setPaused(id, false)
		}
	}
	return errors.Join(errs...)
}

// waitWhileHeld blocks the worker between jobs while the queue is paused.
func (q *Queue) waitWhileHeld() {
	for {
		q.pauseMu.Lock()
		held, ch := q.held, q.resumeCh
		q.pauseMu.Unlock()
		if !held {
			return
		}
		select {
		case <-ch:
		case <-q.stopCh:
			return
		}
	}
}

// Shutdown stops accepting jobs, cancels every unfinished one (a job still
// waiting is marked canceled at once; a running one unwinds through its
// context, which also kills its subprocess tree and removes its temp WAV) and
// waits for the worker goroutine to exit or ctx to expire. Open SSE streams
// get their final "canceled" event, so they end on their own instead of
// holding http.Server.Shutdown open. Safe to call more than once.
func (q *Queue) Shutdown(ctx context.Context) error {
	q.submitMu.Lock()
	if !q.closed {
		q.closed = true
		close(q.stopCh)
		close(q.submit) // Submit sends under submitMu and checks closed first
	}
	q.submitMu.Unlock()

	for _, j := range q.store.List() {
		if !j.Status.IsTerminal() {
			q.Cancel(j.ID)
		}
	}

	select {
	case <-q.workerDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *Queue) run() {
	defer close(q.workerDone)
	for j := range q.submit {
		q.waitWhileHeld()
		q.safeProcess(j)
	}
}

// safeProcess keeps one job's panic (a nil engine, a bug in a callback) from
// taking the whole server down with the worker goroutine.
func (q *Queue) safeProcess(j *Job) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("job worker panic", "job", j.ID, "panic", r)
			q.finishWithError(j.ID, context.Background(), fmt.Errorf("internal error: %v", r))
		}
	}()
	q.process(j)
}

func (q *Queue) process(j *Job) {
	ctx := j.ctx
	defer j.cancel()

	// Canceled while still queued: already marked terminal by Cancel.
	if cur, ok := q.store.Get(j.ID); !ok || cur.Status.IsTerminal() {
		return
	}

	tmpWav := filepath.Join(q.pipeline.TmpDir, j.ID+".wav")
	defer os.Remove(tmpWav)

	if !q.setStatus(j.ID, StatusExtractingAudio, 0, "Extracting audio") {
		return
	}
	// ffprobe is independent of the extraction, so run them side by side; the
	// probe is canceled with the job and when extraction fails first.
	probeCtx, stopProbe := context.WithCancel(ctx)
	defer stopProbe()
	type probeResult struct {
		d   time.Duration
		err error
	}
	probed := make(chan probeResult, 1)
	go func() {
		d, err := q.pipeline.ProbeDuration(probeCtx, j.Request.MediaPath)
		probed <- probeResult{d, err}
	}()
	if err := q.pipeline.ExtractAudio(ctx, j.Request.MediaPath, tmpWav); err != nil {
		q.finishWithError(j.ID, ctx, err)
		return
	}

	// The duration only drives the progress percentage, so a probe failure
	// (e.g. ffprobe reporting N/A for some webm/mkv files) must not fail the
	// job — without it the engine falls back to whisper-cli's own percentage.
	pr := <-probed
	duration, err := pr.d, pr.err
	if err != nil {
		if ctx.Err() != nil {
			q.finishWithError(j.ID, ctx, err)
			return
		}
		slog.Warn("could not probe media duration; progress will rely on whisper-cli's own percentage", "job", j.ID, "error", err)
		duration = 0
	}

	if !q.setStatus(j.ID, StatusTranscribing, 0, "Transcribing") {
		return
	}
	opts := transcribe.Options{
		AudioPath:     tmpWav,
		ModelPath:     j.Request.ModelPath,
		Language:      j.Request.Language,
		OutputPrefix:  OutputPrefix(j.Request),
		MaxLen:        j.Request.MaxLen,
		MaxContext:    j.Request.MaxContext,
		TotalDuration: duration,
		OnStart:       func(c transcribe.Controller) { q.setController(j.ID, c) },
	}
	defer q.setController(j.ID, nil)
	// whisper-cli prints a line per segment, which can be many per second;
	// forward at most one update per progressInterval (the final 100% comes
	// from the Done transition below).
	var lastSent time.Time
	err = q.pipeline.Engine.Transcribe(ctx, opts, func(p transcribe.Progress) {
		if now := time.Now(); now.Sub(lastSent) >= progressInterval {
			lastSent = now
			q.setStatus(j.ID, StatusTranscribing, p.Percent, p.Message)
		}
	})
	if err != nil {
		q.finishWithError(j.ID, ctx, err)
		return
	}

	srtPath := opts.OutputPrefix + ".srt"
	if q.pipeline.PostProcess != nil {
		if err := q.pipeline.PostProcess(srtPath); err != nil {
			slog.Warn("subtitle post-processing failed; keeping whisper-cli's output as is", "job", j.ID, "path", srtPath, "error", err)
		}
	}
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
	canceled := ctx.Err() != nil
	changed := false
	q.store.Update(id, func(j *Job) {
		if j.Status.IsTerminal() {
			return // e.g. already marked canceled by Queue.Cancel
		}
		changed = true
		if canceled {
			j.Status = StatusCanceled
			return
		}
		j.Status = StatusFailed
		j.Error = err.Error()
	})
	if !changed {
		return
	}
	if canceled {
		q.events.Publish(id, "canceled", map[string]string{})
		return
	}
	q.events.Publish(id, "error", map[string]string{"message": err.Error()})
}

// setStatus records a progress update. It returns false, without touching the
// job, if the job already reached a terminal state (a cancel racing the
// worker) — the worker must then stop rather than resurrect it.
func (q *Queue) setStatus(id string, status Status, percent float64, message string) bool {
	ok := false
	q.store.Update(id, func(j *Job) {
		if j.Status.IsTerminal() {
			return
		}
		ok = true
		// Output buffered in the pipe before the process was frozen can still
		// arrive; it must not flip a paused job back to transcribing.
		if j.Status == StatusPaused && status == StatusTranscribing {
			status = StatusPaused
			message = j.Message
		}
		j.Status = status
		j.Percent = percent
		j.Message = message
	})
	if !ok {
		return false
	}
	q.events.Publish(id, "progress", map[string]any{
		"stage":   string(status),
		"percent": percent,
		"message": message,
	})
	return true
}

func newJobID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate job id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
