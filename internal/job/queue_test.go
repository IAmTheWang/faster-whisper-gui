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
