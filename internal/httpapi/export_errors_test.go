package httpapi

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"faster-whisper-gui/internal/job"
)

func TestHandleExportJob_SourceGoneIs404(t *testing.T) {
	store := job.NewStore()
	store.Add(&job.Job{
		ID: "j1", Status: job.StatusDone, CreatedAt: time.Now(),
		SRTPath: filepath.Join(t.TempDir(), "deleted.srt"), // never created
	})
	s := &Server{Store: store}

	rr := exportJob(t, s, "j1", fmt.Sprintf(`{"destDir": %q}`, t.TempDir()))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body %s", rr.Code, rr.Body.String())
	}
}

func TestHandleExportJob_DestinationFailureIs500(t *testing.T) {
	srt := filepath.Join(t.TempDir(), "video.srt")
	if err := os.WriteFile(srt, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A directory already sits where the copy must land, so the final rename
	// fails — a problem on this machine, not a conflict with the request.
	destDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(destDir, "video.srt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "video.srt", "keep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	store := job.NewStore()
	store.Add(&job.Job{ID: "j1", Status: job.StatusDone, SRTPath: srt, CreatedAt: time.Now()})
	s := &Server{Store: store}

	rr := exportJob(t, s, "j1", fmt.Sprintf(`{"destDir": %q}`, destDir))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body %s", rr.Code, rr.Body.String())
	}
}
