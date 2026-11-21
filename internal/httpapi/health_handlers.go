package httpapi

import (
	"context"
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
// change is reflected immediately. A failing result is cached only briefly
// (healthFailureTTL): the user is likely fixing it right now, and the banner
// must not keep showing a problem they already solved.
const (
	healthCacheTTL   = 30 * time.Second
	healthFailureTTL = 3 * time.Second
)

type healthKey struct{ ffmpegPath, whisperCli string }

type healthCall struct {
	done chan struct{}
	resp healthResponse
}

type healthCache struct {
	mu       sync.Mutex
	key      healthKey
	at       time.Time
	resp     healthResponse
	inflight map[healthKey]*healthCall
}

func (r healthResponse) healthy() bool { return r.Ffmpeg.OK && r.WhisperCli.OK }

// do returns the cached result for the path pair, or runs probe — at most once
// at a time per pair: concurrent callers (several tabs loading at once) wait
// for the one in-flight probe instead of each spawning their own subprocesses.
func (c *healthCache) do(ffmpegPath, whisperCli string, probe func() healthResponse) healthResponse {
	key := healthKey{ffmpegPath, whisperCli}
	c.mu.Lock()
	if !c.at.IsZero() && c.key == key {
		ttl := healthCacheTTL
		if !c.resp.healthy() {
			ttl = healthFailureTTL
		}
		if time.Since(c.at) <= ttl {
			resp := c.resp
			c.mu.Unlock()
			return resp
		}
	}
	if call, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-call.done
		return call.resp
	}
	call := &healthCall{done: make(chan struct{})}
	if c.inflight == nil {
		c.inflight = make(map[healthKey]*healthCall)
	}
	c.inflight[key] = call
	c.mu.Unlock()

	call.resp = probe()

	c.mu.Lock()
	c.key, c.at, c.resp = key, time.Now(), call.resp
	delete(c.inflight, key)
	c.mu.Unlock()
	close(call.done)
	return call.resp
}

// handleHealth probes ffmpeg/whisper-cli so missing binaries or DLLs surface
// as a clear message at page load instead of after a user submits a job. The
// two probes run concurrently and results are cached briefly.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ffmpegPath := s.Config.EffectiveFfmpegPath()
	whisperPath := s.Config.EffectiveWhisperCliPath()
	resp := s.health.do(ffmpegPath, whisperPath, func() healthResponse {
		// Not the request's context: the probe is shared with other waiting
		// callers, and one client hanging up must not poison it for them. Each
		// check has its own short timeout.
		ctx := context.Background()
		var resp healthResponse
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); resp.Ffmpeg = transcribe.CheckFfmpeg(ctx, ffmpegPath) }()
		go func() { defer wg.Done(); resp.WhisperCli = transcribe.CheckWhisperCli(ctx, whisperPath) }()
		wg.Wait()
		return resp
	})
	writeJSON(w, http.StatusOK, resp)
}
