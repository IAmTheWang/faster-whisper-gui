package job

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"faster-whisper-gui/internal/transcribe"
)

type fakeController struct {
	mu                sync.Mutex
	suspends, resumes int
	err               error
}

func (c *fakeController) Suspend() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.suspends++
	return nil
}

func (c *fakeController) Resume() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resumes++
	return nil
}

// pausableEngine hands a controller to the queue and then blocks until
// released (or canceled), optionally reporting progress first.
type pausableEngine struct {
	ctl     *fakeController
	release chan struct{}
	started chan struct{}
	once    sync.Once
}

func (e *pausableEngine) Transcribe(ctx context.Context, opts transcribe.Options, onProgress func(transcribe.Progress)) error {
	opts.OnStart(e.ctl)
	e.once.Do(func() { close(e.started) })
	select {
	case <-e.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	onProgress(transcribe.Progress{Percent: 50, Message: "late"})
	return nil
}

func newPausableQueue(t *testing.T) (*Queue, *Store, *pausableEngine, *fakePublisher) {
	t.Helper()
	store := NewStore()
	pub := &fakePublisher{}
	eng := &pausableEngine{ctl: &fakeController{}, release: make(chan struct{}), started: make(chan struct{})}
	q := NewQueue(store, Pipeline{
		ExtractAudio:  noopFfmpeg,
		ProbeDuration: fixedDuration(time.Second),
		TmpDir:        t.TempDir(),
		Engine:        eng,
	}, pub)
	return q, store, eng, pub
}

func TestPauseAndResumeTranscribingJob(t *testing.T) {
	q, store, eng, pub := newPausableQueue(t)
	j, err := q.Submit(Request{MediaPath: `E:\v\1.mp4`, OutputMode: OutputSameAsSource})
	if err != nil {
		t.Fatal(err)
	}
	<-eng.started
	waitForStatus(t, store, j.ID, StatusTranscribing, 2*time.Second)

	if err := q.Resume(j.ID); !errors.Is(err, ErrNotPaused) {
		t.Fatalf("Resume of a running job = %v, want ErrNotPaused", err)
	}
	if err := q.Pause(j.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if got, _ := store.Get(j.ID); got.Status != StatusPaused {
		t.Fatalf("status = %s, want paused", got.Status)
	}
	if err := q.Pause(j.ID); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("second Pause = %v, want ErrNotRunning", err)
	}

	// Output that was already buffered must not flip the job back to running.
	q.setStatus(j.ID, StatusTranscribing, 30, "buffered segment")
	got, _ := store.Get(j.ID)
	if got.Status != StatusPaused || got.Percent != 30 {
		t.Fatalf("after late progress: status=%s percent=%v, want paused/30", got.Status, got.Percent)
	}

	if err := q.Resume(j.ID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if got, _ := store.Get(j.ID); got.Status != StatusTranscribing {
		t.Fatalf("status after resume = %s, want transcribing", got.Status)
	}

	close(eng.release)
	waitForStatus(t, store, j.ID, StatusDone, 2*time.Second)
	if eng.ctl.suspends != 1 || eng.ctl.resumes != 1 {
		t.Fatalf("suspends=%d resumes=%d, want 1/1", eng.ctl.suspends, eng.ctl.resumes)
	}
	if err := q.Pause(j.ID); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Pause of a finished job = %v, want ErrNotRunning", err)
	}
	// The pause and the resume each published a progress event.
	progress := 0
	for _, ty := range pub.eventTypesFor(j.ID) {
		if ty == "progress" {
			progress++
		}
	}
	if progress < 2 {
		t.Fatalf("expected progress events for pause and resume, got %v", pub.eventTypesFor(j.ID))
	}
}

func TestCancelPausedJob(t *testing.T) {
	q, store, eng, _ := newPausableQueue(t)
	j, _ := q.Submit(Request{MediaPath: `E:\v\1.mp4`, OutputMode: OutputSameAsSource})
	<-eng.started
	waitForStatus(t, store, j.ID, StatusTranscribing, 2*time.Second)
	if err := q.Pause(j.ID); err != nil {
		t.Fatal(err)
	}
	if !q.Cancel(j.ID) {
		t.Fatal("Cancel of a paused job returned false")
	}
	waitForStatus(t, store, j.ID, StatusCanceled, 2*time.Second)
}

func TestPauseFailureLeavesJobRunning(t *testing.T) {
	q, store, eng, _ := newPausableQueue(t)
	eng.ctl.err = errors.New("access denied")
	j, _ := q.Submit(Request{MediaPath: `E:\v\1.mp4`, OutputMode: OutputSameAsSource})
	<-eng.started
	waitForStatus(t, store, j.ID, StatusTranscribing, 2*time.Second)
	if err := q.Pause(j.ID); err == nil || errors.Is(err, ErrNotRunning) {
		t.Fatalf("Pause = %v, want the controller's error", err)
	}
	if got, _ := store.Get(j.ID); got.Status != StatusTranscribing {
		t.Fatalf("status = %s, want transcribing", got.Status)
	}
	close(eng.release)
	waitForStatus(t, store, j.ID, StatusDone, 2*time.Second)
}

func TestPauseUnknownOrQueuedJob(t *testing.T) {
	q, _, _, _ := newPausableQueue(t)
	if err := q.Pause("nope"); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Pause(unknown) = %v", err)
	}
	if err := q.Resume("nope"); !errors.Is(err, ErrNotPaused) {
		t.Fatalf("Resume(unknown) = %v", err)
	}
}

func TestPauseAllFreezesRunningJobAndHoldsTheNext(t *testing.T) {
	q, store, eng, _ := newPausableQueue(t)
	j1, _ := q.Submit(Request{MediaPath: `E:\v\1.mp4`, OutputMode: OutputSameAsSource})
	j2, _ := q.Submit(Request{MediaPath: `E:\v\2.mp4`, OutputMode: OutputSameAsSource})
	<-eng.started
	waitForStatus(t, store, j1.ID, StatusTranscribing, 2*time.Second)

	if err := q.PauseAll(); err != nil {
		t.Fatal(err)
	}
	if !q.Held() {
		t.Fatal("Held = false after PauseAll")
	}
	if got, _ := store.Get(j1.ID); got.Status != StatusPaused {
		t.Fatalf("running job status = %s, want paused", got.Status)
	}

	// Release the engine; even then job 1 stays frozen (the fake ignores the
	// freeze, so it finishes) and job 2 must not start while held.
	close(eng.release)
	waitForStatus(t, store, j1.ID, StatusDone, 2*time.Second)
	time.Sleep(100 * time.Millisecond)
	if got, _ := store.Get(j2.ID); got.Status != StatusQueued {
		t.Fatalf("second job status = %s while queue is held, want queued", got.Status)
	}

	if err := q.ResumeAll(); err != nil {
		t.Fatal(err)
	}
	if q.Held() {
		t.Fatal("Held = true after ResumeAll")
	}
	waitForStatus(t, store, j2.ID, StatusDone, 2*time.Second)
}

func TestResumeAllThawsPausedJob(t *testing.T) {
	q, store, eng, _ := newPausableQueue(t)
	j, _ := q.Submit(Request{MediaPath: `E:\v\1.mp4`, OutputMode: OutputSameAsSource})
	<-eng.started
	waitForStatus(t, store, j.ID, StatusTranscribing, 2*time.Second)
	_ = q.PauseAll()
	if err := q.ResumeAll(); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(j.ID); got.Status != StatusTranscribing {
		t.Fatalf("status = %s, want transcribing", got.Status)
	}
	close(eng.release)
	waitForStatus(t, store, j.ID, StatusDone, 2*time.Second)
}

func TestShutdownReleasesHeldWorker(t *testing.T) {
	q, _, _, _ := newPausableQueue(t)
	_ = q.PauseAll()
	_, _ = q.Submit(Request{MediaPath: `E:\v\1.mp4`, OutputMode: OutputSameAsSource})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := q.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown with a held queue: %v", err)
	}
}

// A queue paused while a job is still extracting audio freezes that job the
// moment its engine starts, and ResumeAll thaws it again.
func TestPauseAllDuringExtractionFreezesOnEngineStart(t *testing.T) {
	store := NewStore()
	extracting := make(chan struct{})
	proceed := make(chan struct{})
	eng := &pausableEngine{ctl: &fakeController{}, release: make(chan struct{}), started: make(chan struct{})}
	q := NewQueue(store, Pipeline{
		ExtractAudio: func(ctx context.Context, _, _ string) error {
			close(extracting)
			<-proceed
			return nil
		},
		ProbeDuration: fixedDuration(time.Second),
		TmpDir:        t.TempDir(),
		Engine:        eng,
	}, &fakePublisher{})
	j, _ := q.Submit(Request{MediaPath: `E:\v\1.mp4`, OutputMode: OutputSameAsSource})
	<-extracting
	if err := q.PauseAll(); err != nil {
		t.Fatal(err)
	}
	close(proceed)
	<-eng.started
	waitForStatus(t, store, j.ID, StatusPaused, 2*time.Second)
	if err := q.ResumeAll(); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(j.ID); got.Status != StatusTranscribing {
		t.Fatalf("status = %s, want transcribing", got.Status)
	}
	close(eng.release)
	waitForStatus(t, store, j.ID, StatusDone, 2*time.Second)
}

// A controller reporting "process gone" (the job is finishing) must surface as
// the same conflict errors as any other not-pausable state, never as a failure.
type goneController struct{}

func (goneController) Suspend() error { return transcribe.ErrProcessGone }
func (goneController) Resume() error  { return transcribe.ErrProcessGone }

func TestProcessGoneIsAConflictNotAFailure(t *testing.T) {
	q, store, eng, _ := newPausableQueue(t)
	j, _ := q.Submit(Request{MediaPath: `E:\v\1.mp4`, OutputMode: OutputSameAsSource})
	<-eng.started
	waitForStatus(t, store, j.ID, StatusTranscribing, 2*time.Second)
	q.setController(j.ID, goneController{})

	if err := q.Pause(j.ID); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Pause = %v, want ErrNotRunning", err)
	}
	if err := q.PauseAll(); err != nil {
		t.Fatalf("PauseAll = %v, want nil for a process that is already gone", err)
	}
	if err := q.ResumeAll(); err != nil {
		t.Fatalf("ResumeAll = %v, want nil", err)
	}
	close(eng.release)
	waitForStatus(t, store, j.ID, StatusDone, 2*time.Second)
}
