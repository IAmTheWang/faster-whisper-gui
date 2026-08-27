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

	"faster-whisper-gui/internal/config"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg, err := config.Load(t.TempDir())
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return &Server{Config: cfg}
}

func mustWriteExe(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("fake exe"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func putSettings(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleUpdateSettings(rr, req)
	return rr
}

func getSettings(t *testing.T, s *Server) settingsResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetSettings(rr, req)
	var resp settingsResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}
	return resp
}

func TestHandleUpdateSettings_OmittedFieldLeftUnchanged(t *testing.T) {
	s := newTestServer(t)
	ffmpeg := filepath.Join(t.TempDir(), "ffmpeg.exe")
	mustWriteExe(t, ffmpeg)
	whisper := filepath.Join(t.TempDir(), "whisper-cli.exe")
	mustWriteExe(t, whisper)

	// First call sets ffmpegPath only.
	rr := putSettings(t, s, fmt.Sprintf(`{"ffmpegPath": %q}`, ffmpeg))
	if rr.Code != http.StatusOK {
		t.Fatalf("first PUT: status %d, body %s", rr.Code, rr.Body.String())
	}

	// Second call sets whisperCliPath only, omitting ffmpegPath entirely.
	rr = putSettings(t, s, fmt.Sprintf(`{"whisperCliPath": %q}`, whisper))
	if rr.Code != http.StatusOK {
		t.Fatalf("second PUT: status %d, body %s", rr.Code, rr.Body.String())
	}

	got := getSettings(t, s)
	if got.FfmpegPath != filepath.Clean(ffmpeg) {
		t.Errorf("FfmpegPath = %q, want it left unchanged at %q", got.FfmpegPath, ffmpeg)
	}
	if got.WhisperCliPath != filepath.Clean(whisper) {
		t.Errorf("WhisperCliPath = %q, want %q", got.WhisperCliPath, whisper)
	}
}

func TestHandleUpdateSettings_EmptyStringClearsOverride(t *testing.T) {
	s := newTestServer(t)
	ffmpeg := filepath.Join(t.TempDir(), "ffmpeg.exe")
	mustWriteExe(t, ffmpeg)

	if rr := putSettings(t, s, fmt.Sprintf(`{"ffmpegPath": %q}`, ffmpeg)); rr.Code != http.StatusOK {
		t.Fatalf("set: status %d, body %s", rr.Code, rr.Body.String())
	}
	if got := getSettings(t, s).FfmpegPath; got != filepath.Clean(ffmpeg) {
		t.Fatalf("FfmpegPath after set = %q, want %q", got, ffmpeg)
	}

	if rr := putSettings(t, s, `{"ffmpegPath": ""}`); rr.Code != http.StatusOK {
		t.Fatalf("clear: status %d, body %s", rr.Code, rr.Body.String())
	}
	if got := getSettings(t, s).FfmpegPath; got != "" {
		t.Errorf("FfmpegPath after clearing = %q, want empty (back to default)", got)
	}
}

func TestHandleUpdateSettings_DefaultVideoDirRoundtrip(t *testing.T) {
	s := newTestServer(t)
	videoDir := t.TempDir()

	if rr := putSettings(t, s, fmt.Sprintf(`{"defaultVideoDir": %q}`, videoDir)); rr.Code != http.StatusOK {
		t.Fatalf("set: status %d, body %s", rr.Code, rr.Body.String())
	}
	if got := getSettings(t, s).DefaultVideoDir; got != filepath.Clean(videoDir) {
		t.Fatalf("DefaultVideoDir after set = %q, want %q", got, videoDir)
	}

	if rr := putSettings(t, s, `{"defaultVideoDir": ""}`); rr.Code != http.StatusOK {
		t.Fatalf("clear: status %d, body %s", rr.Code, rr.Body.String())
	}
	if got := getSettings(t, s).DefaultVideoDir; got != "" {
		t.Errorf("DefaultVideoDir after clearing = %q, want empty", got)
	}
}

func TestHandleUpdateSettings_InvalidPathRejected(t *testing.T) {
	s := newTestServer(t)

	rr := putSettings(t, s, `{"ffmpegPath": "C:\\does\\not\\exist.exe"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if got := getSettings(t, s).FfmpegPath; got != "" {
		t.Errorf("FfmpegPath after rejected update = %q, want still empty", got)
	}
}
