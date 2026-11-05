package job

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"faster-whisper-gui/internal/transcribe"
)

// fakeEngine lets tests control transcription timing/outcome without a real
// whisper-cli process, and records how many calls were in flight at once so
// tests can assert the queue never runs two jobs concurrently.
type fakeEngine struct {
	mu         sync.Mutex
	running    int
	maxRunning int
	delay      time.Duration
	err        error
}

func (f *fakeEngine) Transcribe(ctx context.Context, opts transcribe.Options, onProgress func(transcribe.Progress)) error {
	f.mu.Lock()
	f.running++
	if f.running > f.maxRunning {
		f.maxRunning = f.running
	}
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		f.running--
		f.mu.Unlock()
	}()

	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if onProgress != nil {
		onProgress(transcribe.Progress{Percent: 100})
	}
	return f.err
}

// fakePublisher records every published event for assertions.
type fakePublisher struct {
	mu     sync.Mutex
	events []publishedEvent
}

type publishedEvent struct {
	jobID string
	typ   string
	data  any
}

func (p *fakePublisher) Publish(jobID string, eventType string, data any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, publishedEvent{jobID, eventType, data})
}

func (p *fakePublisher) eventTypesFor(jobID string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, e := range p.events {
		if e.jobID == jobID {
			out = append(out, e.typ)
		}
	}
	return out
}

func noopFfmpeg(ctx context.Context, mediaPath, outWavPath string) error { return nil }

func fixedDuration(d time.Duration) func(context.Context, string) (time.Duration, error) {
	return func(ctx context.Context, mediaPath string) (time.Duration, error) { return d, nil }
}

func waitForStatus(t *testing.T, store *Store, id string, want Status, timeout time.Duration) Job {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if j, ok := store.Get(id); ok && j.Status == want {
			return j
		}
		if time.Now().After(deadline) {
			j, _ := store.Get(id)
			t.Fatalf("job %s did not reach status %s within %s (last status: %s)", id, want, timeout, j.Status)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestQueueRunsJobsSequentially(t *testing.T) {
	store := NewStore()
	pub := &fakePublisher{}
	engine := &fakeEngine{delay: 100 * time.Millisecond}
	pipeline := Pipeline{
		ExtractAudio:  noopFfmpeg,
		ProbeDuration: fixedDuration(time.Second),
		TmpDir:        t.TempDir(),
		Engine:        engine,
	}
	q := NewQueue(store, pipeline, pub)

	j1, err := q.Submit(Request{MediaPath: `E:\v\1.mp4`, OutputMode: OutputSameAsSource})
	if err != nil {
		t.Fatalf("Submit(1): %v", err)
	}
	j2, err := q.Submit(Request{MediaPath: `E:\v\2.mp4`, OutputMode: OutputSameAsSource})
	if err != nil {
		t.Fatalf("Submit(2): %v", err)
	}

	waitForStatus(t, store, j2.ID, StatusDone, 2*time.Second)
	j1Final := waitForStatus(t, store, j1.ID, StatusDone, 2*time.Second)
	if j1Final.SRTPath == "" {
		t.Error("expected SRTPath to be set on a done job")
	}

	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.maxRunning > 1 {
		t.Fatalf("maxRunning = %d, want at most 1 (jobs must run one at a time)", engine.maxRunning)
	}
}

func TestQueueCancel(t *testing.T) {
	store := NewStore()
	pub := &fakePublisher{}
	engine := &fakeEngine{delay: 5 * time.Second}
	pipeline := Pipeline{
		ExtractAudio:  noopFfmpeg,
		ProbeDuration: fixedDuration(time.Second),
		TmpDir:        t.TempDir(),
		Engine:        engine,
	}
	q := NewQueue(store, pipeline, pub)

	j, err := q.Submit(Request{MediaPath: `E:\v\1.mp4`, OutputMode: OutputSameAsSource})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	waitForStatus(t, store, j.ID, StatusTranscribing, 2*time.Second)

	if !store.Cancel(j.ID) {
		t.Fatal("Cancel returned false")
	}

	waitForStatus(t, store, j.ID, StatusCanceled, 2*time.Second)

	found := false
	for _, ty := range pub.eventTypesFor(j.ID) {
		if ty == "canceled" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a %q event, got %v", "canceled", pub.eventTypesFor(j.ID))
	}
}

func TestQueueEngineFailure(t *testing.T) {
	store := NewStore()
	pub := &fakePublisher{}
	engine := &fakeEngine{err: errors.New("boom")}
	pipeline := Pipeline{
		ExtractAudio:  noopFfmpeg,
		ProbeDuration: fixedDuration(time.Second),
		TmpDir:        t.TempDir(),
		Engine:        engine,
	}
	q := NewQueue(store, pipeline, pub)

	j, err := q.Submit(Request{MediaPath: `E:\v\1.mp4`, OutputMode: OutputSameAsSource})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	final := waitForStatus(t, store, j.ID, StatusFailed, 2*time.Second)
	if final.Error == "" {
		t.Error("expected Error to be set on a failed job")
	}

	found := false
	for _, ty := range pub.eventTypesFor(j.ID) {
		if ty == "error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an %q event, got %v", "error", pub.eventTypesFor(j.ID))
	}
}

func TestQueueCancelWhileQueued(t *testing.T) {
	store := NewStore()
	pub := &fakePublisher{}
	engine := &fakeEngine{delay: 5 * time.Second}
	pipeline := Pipeline{
		ExtractAudio:  noopFfmpeg,
		ProbeDuration: fixedDuration(time.Second),
		TmpDir:        t.TempDir(),
		Engine:        engine,
	}
	q := NewQueue(store, pipeline, pub)

	running, _ := q.Submit(Request{MediaPath: `E:\v\1.mp4`, OutputMode: OutputSameAsSource})
	waiting, _ := q.Submit(Request{MediaPath: `E:\v\2.mp4`, OutputMode: OutputSameAsSource})
	waitForStatus(t, store, running.ID, StatusTranscribing, 2*time.Second)

	if !q.Cancel(waiting.ID) {
		t.Fatal("Cancel on a queued job returned false")
	}
	waitForStatus(t, store, waiting.ID, StatusCanceled, time.Second)

	// Let the first job finish; the canceled one must never start.
	q.Cancel(running.ID)
	waitForStatus(t, store, running.ID, StatusCanceled, 2*time.Second)
	time.Sleep(50 * time.Millisecond)
	if j, _ := store.Get(waiting.ID); j.Status != StatusCanceled {
		t.Fatalf("queued-canceled job ended as %s, want canceled", j.Status)
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.maxRunning != 1 {
		t.Fatalf("maxRunning = %d, want 1 (canceled job must not run)", engine.maxRunning)
	}
}

type panicEngine struct{}

func (panicEngine) Transcribe(context.Context, transcribe.Options, func(transcribe.Progress)) error {
	panic("kaboom")
}

func TestQueueSurvivesWorkerPanic(t *testing.T) {
	store := NewStore()
	pub := &fakePublisher{}
	pipeline := Pipeline{
		ExtractAudio:  noopFfmpeg,
		ProbeDuration: fixedDuration(time.Second),
		TmpDir:        t.TempDir(),
		Engine:        panicEngine{},
	}
	q := NewQueue(store, pipeline, pub)

	j1, _ := q.Submit(Request{MediaPath: `E:\v\1.mp4`, OutputMode: OutputSameAsSource})
	waitForStatus(t, store, j1.ID, StatusFailed, 2*time.Second)

	// The worker goroutine must still be alive for the next job.
	j2, _ := q.Submit(Request{MediaPath: `E:\v\2.mp4`, OutputMode: OutputSameAsSource})
	waitForStatus(t, store, j2.ID, StatusFailed, 2*time.Second)
}

func TestQueueProbeFailureIsNotFatal(t *testing.T) {
	store := NewStore()
	pub := &fakePublisher{}
	pipeline := Pipeline{
		ExtractAudio: noopFfmpeg,
		ProbeDuration: func(context.Context, string) (time.Duration, error) {
			return 0, errors.New("N/A")
		},
		TmpDir: t.TempDir(),
		Engine: &fakeEngine{},
	}
	q := NewQueue(store, pipeline, pub)

	j, _ := q.Submit(Request{MediaPath: `E:\v\1.mp4`, OutputMode: OutputSameAsSource})
	waitForStatus(t, store, j.ID, StatusDone, 2*time.Second)
}

func TestQueueFullReturnsErrQueueFull(t *testing.T) {
	store := NewStore()
	block := make(chan struct{})
	pipeline := Pipeline{
		ExtractAudio: func(ctx context.Context, _, _ string) error {
			select {
			case <-block:
			case <-ctx.Done():
			}
			return nil
		},
		ProbeDuration: fixedDuration(time.Second),
		TmpDir:        t.TempDir(),
		Engine:        &fakeEngine{},
	}
	q := NewQueue(store, pipeline, &fakePublisher{})
	defer close(block)

	var fullErr error
	for i := 0; i < 200 && fullErr == nil; i++ {
		_, fullErr = q.Submit(Request{MediaPath: `E:\v\x.mp4`, OutputMode: OutputSameAsSource})
	}
	if !errors.Is(fullErr, ErrQueueFull) {
		t.Fatalf("Submit never reported ErrQueueFull, last err = %v", fullErr)
	}
	// The rejected job must not linger in the store as a phantom "queued" job.
	for _, j := range store.List() {
		if j.Status == StatusQueued && j.ctx.Err() != nil {
			t.Fatal("rejected job left in store")
		}
	}
}

func TestQueuePostProcessRunsBeforeDoneAndNeverFailsTheJob(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{{"ok", nil}, {"error is non-fatal", errors.New("disk full")}} {
		t.Run(tc.name, func(t *testing.T) {
			store := NewStore()
			var gotPath string
			var statusAtCall Status
			pipeline := Pipeline{
				ExtractAudio:  noopFfmpeg,
				ProbeDuration: fixedDuration(time.Second),
				TmpDir:        t.TempDir(),
				Engine:        &fakeEngine{},
			}
			var jobID string
			pipeline.PostProcess = func(srtPath string) error {
				gotPath = srtPath
				j, _ := store.Get(jobID)
				statusAtCall = j.Status
				return tc.err
			}
			q := NewQueue(store, pipeline, &fakePublisher{})
			j, _ := q.Submit(Request{MediaPath: `E:\v\clip.mp4`, OutputMode: OutputSameAsSource})
			jobID = j.ID

			final := waitForStatus(t, store, j.ID, StatusDone, 2*time.Second)
			if gotPath != `E:\v\clip.srt` || final.SRTPath != gotPath {
				t.Errorf("PostProcess path = %q, SRTPath = %q", gotPath, final.SRTPath)
			}
			if statusAtCall.IsTerminal() {
				t.Errorf("PostProcess must run before the job is marked done, status was %s", statusAtCall)
			}
		})
	}
}

func TestQueuePostProcessSkippedWhenTranscriptionFails(t *testing.T) {
	store := NewStore()
	called := false
	q := NewQueue(store, Pipeline{
		ExtractAudio:  noopFfmpeg,
		ProbeDuration: fixedDuration(time.Second),
		TmpDir:        t.TempDir(),
		Engine:        &fakeEngine{err: errors.New("boom")},
		PostProcess:   func(string) error { called = true; return nil },
	}, &fakePublisher{})
	j, _ := q.Submit(Request{MediaPath: `E:\v\clip.mp4`, OutputMode: OutputSameAsSource})
	waitForStatus(t, store, j.ID, StatusFailed, 2*time.Second)
	if called {
		t.Fatal("PostProcess must not run for a failed job")
	}
}
