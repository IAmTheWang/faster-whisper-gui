package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"faster-whisper-gui/internal/job"
)

func exportJob(t *testing.T, s *Server, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/"+id+"/export", strings.NewReader(body))
	req.SetPathValue("id", id)
	rr := httptest.NewRecorder()
	s.handleExportJob(rr, req)
	return rr
}

func TestHandleExportJob_NotFound(t *testing.T) {
	s := &Server{Store: job.NewStore()}
	rr := exportJob(t, s, "does-not-exist", `{"destDir": "C:\\anything"}`)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body %s", rr.Code, rr.Body.String())
	}
}

func TestHandleExportJob_NotDoneYet(t *testing.T) {
	store := job.NewStore()
	store.Add(&job.Job{ID: "j1", Status: job.StatusTranscribing, CreatedAt: time.Now()})
	s := &Server{Store: store}

	rr := exportJob(t, s, "j1", `{"destDir": "C:\\anything"}`)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body %s", rr.Code, rr.Body.String())
	}
}

func TestHandleExportJob_InvalidDestDir(t *testing.T) {
	dir := t.TempDir()
	srt := filepath.Join(dir, "video.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:01,000\nhi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := job.NewStore()
	store.Add(&job.Job{ID: "j1", Status: job.StatusDone, SRTPath: srt, CreatedAt: time.Now()})
	s := &Server{Store: store}

	rr := exportJob(t, s, "j1", `{"destDir": "C:\\does\\not\\exist"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body %s", rr.Code, rr.Body.String())
	}
}

func TestHandleExportJob_Success(t *testing.T) {
	srcDir := t.TempDir()
	srt := filepath.Join(srcDir, "video.srt")
	content := "1\n00:00:00,000 --> 00:00:01,000\nhi\n"
	if err := os.WriteFile(srt, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	destDir := t.TempDir()

	store := job.NewStore()
	store.Add(&job.Job{ID: "j1", Status: job.StatusDone, SRTPath: srt, CreatedAt: time.Now()})
	s := &Server{Store: store}

	rr := exportJob(t, s, "j1", fmt.Sprintf(`{"destDir": %q}`, destDir))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body %s", rr.Code, rr.Body.String())
	}

	var resp exportJobResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	wantPath := filepath.Join(destDir, "video.srt")
	if resp.DestPath != wantPath {
		t.Errorf("DestPath = %q, want %q", resp.DestPath, wantPath)
	}

	got, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	if string(got) != content {
		t.Errorf("exported content = %q, want %q", got, content)
	}

	if _, err := os.Stat(srt); err != nil {
		t.Errorf("original SRT should be untouched: %v", err)
	}
}
