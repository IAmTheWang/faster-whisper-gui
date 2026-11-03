package httpapi

import (
	"net/http"
	"sync"
	"time"

	"faster-whisper-gui/internal/transcribe"
)

type healthResponse struct {
	Ffmpeg     transcribe.ComponentHealth `json:"ffmpeg"`
	WhisperCli transcribe.ComponentHealth `json:"whisperCli"`
}

// healthCacheTTL keeps repeated page loads from spawning two subprocesses
// each time. The cache is keyed on the resolved binary paths, so a Settings
// change is reflected immediately.
const healthCacheTTL = 30 * time.Second

type healthCache struct {
	mu         sync.Mutex
	ffmpegPath string
	whisperCli string
	at         time.Time
	resp       healthResponse
}

func (c *healthCache) get(ffmpegPath, whisperCli string) (healthResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.at.IsZero() || c.ffmpegPath != ffmpegPath || c.whisperCli != whisperCli || time.Since(c.at) > healthCacheTTL {
		return healthResponse{}, false
	}
	return c.resp, true
}

func (c *healthCache) put(ffmpegPath, whisperCli string, resp healthResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ffmpegPath, c.whisperCli, c.at, c.resp = ffmpegPath, whisperCli, time.Now(), resp
}

// handleHealth probes ffmpeg/whisper-cli so missing binaries or DLLs surface
// as a clear message at page load instead of after a user submits a job. The
// two probes run concurrently and results are cached briefly.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ffmpegPath := s.Config.EffectiveFfmpegPath()
	whisperPath := s.Config.EffectiveWhisperCliPath()
	if resp, ok := s.health.get(ffmpegPath, whisperPath); ok {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	ctx := r.Context()
	var resp healthResponse
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); resp.Ffmpeg = transcribe.CheckFfmpeg(ctx, ffmpegPath) }()
	go func() { defer wg.Done(); resp.WhisperCli = transcribe.CheckWhisperCli(ctx, whisperPath) }()
	wg.Wait()

	if ctx.Err() == nil { // don't cache a probe the client abandoned
		s.health.put(ffmpegPath, whisperPath, resp)
	}
	writeJSON(w, http.StatusOK, resp)
}
