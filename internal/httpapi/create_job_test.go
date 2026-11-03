package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"faster-whisper-gui/internal/config"
	"faster-whisper-gui/internal/job"
	"faster-whisper-gui/internal/sseutil"
	"faster-whisper-gui/internal/transcribe"
)

// blockingEngine keeps the first job "running" so later submissions stay
// unfinished (queued) for the duration of a test.
type blockingEngine struct{ release chan struct{} }

func (e blockingEngine) Transcribe(ctx context.Context, _ transcribe.Options, _ func(transcribe.Progress)) error {
	select {
	case <-e.release:
	case <-ctx.Done():
	}
	return nil
}

func newCreateJobServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.ModelsDir, "ggml-tiny.bin"), []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(root, "clip.wav")
	if err := os.WriteFile(media, []byte("w"), 0o644); err != nil {
		t.Fatal(err)
	}

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	store := job.NewStore()
	broker := sseutil.NewBroker()
	queue := job.NewQueue(store, job.Pipeline{
		ExtractAudio:  func(context.Context, string, string) error { return nil },
		ProbeDuration: func(context.Context, string) (time.Duration, error) { return time.Second, nil },
		TmpDir:        cfg.TmpDir,
		Engine:        blockingEngine{release},
	}, broker)
	return NewServer(cfg, store, queue, broker), media
}

func postJob(s *Server, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateJob(rr, req)
	return rr
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestHandleCreateJob(t *testing.T) {
	s, media := newCreateJobServer(t)
	ok := `{"mediaPath":` + jsonString(media) + `,"modelId":"tiny","language":"en"}`

	t.Run("bad json", func(t *testing.T) {
		if rr := postJob(s, `{`); rr.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", rr.Code)
		}
	})
	t.Run("missing media", func(t *testing.T) {
		body := `{"mediaPath":` + jsonString(media+".nope") + `,"modelId":"tiny"}`
		if rr := postJob(s, body); rr.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400: %s", rr.Code, rr.Body)
		}
	})
	t.Run("unknown model", func(t *testing.T) {
		body := `{"mediaPath":` + jsonString(media) + `,"modelId":"huge"}`
		if rr := postJob(s, body); rr.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400: %s", rr.Code, rr.Body)
		}
	})
	t.Run("flag-like language", func(t *testing.T) {
		body := `{"mediaPath":` + jsonString(media) + `,"modelId":"tiny","language":"--help"}`
		if rr := postJob(s, body); rr.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400: %s", rr.Code, rr.Body)
		}
	})
	t.Run("unknown output mode", func(t *testing.T) {
		body := `{"mediaPath":` + jsonString(media) + `,"modelId":"tiny","outputMode":"elsewhere"}`
		if rr := postJob(s, body); rr.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400: %s", rr.Code, rr.Body)
		}
	})

	var firstID string
	t.Run("accepted and leaves no stray srt", func(t *testing.T) {
		rr := postJob(s, ok)
		if rr.Code != http.StatusAccepted || !strings.Contains(rr.Body.String(), `"status":"queued"`) {
			t.Fatalf("got %d %s, want 202 queued", rr.Code, rr.Body)
		}
		firstID = s.Store.List()[0].ID
		if _, err := os.Stat(strings.TrimSuffix(media, ".wav") + ".srt"); err == nil {
			t.Error("submission must not leave an empty .srt behind")
		}
	})
	t.Run("same srt while unfinished conflicts", func(t *testing.T) {
		if rr := postJob(s, ok); rr.Code != http.StatusConflict {
			t.Fatalf("got %d, want 409: %s", rr.Code, rr.Body)
		}
	})
	t.Run("cancel then resubmit works", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/jobs/"+firstID+"/cancel", nil)
		req.SetPathValue("id", firstID)
		rr := httptest.NewRecorder()
		s.handleCancelJob(rr, req)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("cancel got %d, want 202", rr.Code)
		}
		// Cancel returns before the worker finishes unwinding; wait for terminal.
		deadline := time.Now().Add(2 * time.Second)
		for {
			if j, _ := s.Store.Get(firstID); j.Status.IsTerminal() || time.Now().After(deadline) {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if rr := postJob(s, ok); rr.Code != http.StatusAccepted {
			t.Fatalf("resubmit got %d, want 202: %s", rr.Code, rr.Body)
		}
	})
}

func TestHandleCancelJobFinishedIsConflict(t *testing.T) {
	s, _ := newCreateJobServer(t)
	s.Store.Add(&job.Job{ID: "done1", Status: job.StatusDone})
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/done1/cancel", nil)
	req.SetPathValue("id", "done1")
	rr := httptest.NewRecorder()
	s.handleCancelJob(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("got %d, want 409", rr.Code)
	}
}
