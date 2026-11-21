// Package appserver assembles the fully-wired HTTP handler (API routes +
// embedded frontend, with the SPA fallback and middleware applied) shared
// by cmd/server (the production single-binary build) and cmd/dev (which
// pairs the same Go backend with the Vite dev server), so the two
// entrypoints can't drift apart.
package appserver

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"faster-whisper-gui/internal/config"
	"faster-whisper-gui/internal/httpapi"
	"faster-whisper-gui/internal/job"
	"faster-whisper-gui/internal/sseutil"
	"faster-whisper-gui/internal/subtitle"
	"faster-whisper-gui/internal/transcribe"
	"faster-whisper-gui/internal/webui"
)

// App is the assembled backend: an *http.Server plus the job queue behind it.
// It embeds the server, so Serve/ListenAndServe work as usual; Shutdown and
// Close additionally cancel the unfinished jobs first, so a transcription in
// flight ends cleanly (its temp WAV removed, its SSE streams closed) instead
// of only dying when the process exits.
type App struct {
	*http.Server
	queue *job.Queue
}

// Shutdown cancels all jobs, then gracefully shuts the HTTP server down. Both
// steps share ctx's deadline.
func (a *App) Shutdown(ctx context.Context) error {
	if err := a.queue.Shutdown(ctx); err != nil {
		slog.Warn("job queue did not stop in time", "error", err)
	}
	return a.Server.Shutdown(ctx)
}

// Close cancels all jobs (waiting briefly for them to unwind) and then closes
// the HTTP server immediately, dropping open connections.
func (a *App) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = a.queue.Shutdown(ctx)
	return a.Server.Close()
}

// New builds the backend for cfg, ready to Serve/ListenAndServe. It does not
// start listening — callers own that, so cmd/dev can run it in a goroutine
// and Shutdown it later alongside the Vite dev server it launches.
func New(cfg *config.Config) (*App, error) {
	if err := requireLoopbackAddr(cfg.Addr); err != nil {
		return nil, err
	}
	distFS, err := fs.Sub(webui.Dist, "dist")
	if err != nil {
		return nil, fmt.Errorf("mount embedded frontend: %w", err)
	}

	store := job.NewStore()
	broker := sseutil.NewBroker()
	engine := &transcribe.WhisperCppEngine{CliPath: cfg.EffectiveWhisperCliPath}
	pipeline := job.Pipeline{
		ExtractAudio: func(ctx context.Context, mediaPath, outWavPath string) error {
			return transcribe.ExtractAudio(ctx, cfg.EffectiveFfmpegPath(), mediaPath, outWavPath)
		},
		ProbeDuration: func(ctx context.Context, mediaPath string) (time.Duration, error) {
			return transcribe.ProbeDuration(ctx, cfg.EffectiveFfprobePath(), mediaPath)
		},
		TmpDir: cfg.TmpDir,
		Engine: engine,
		// whisper hallucinates runaway repetition on silence/noise and masks
		// profanity ("f*cking"); trim repeats to subtitle.MaxRepeat and
		// spell masked words out in the saved .srt.
		PostProcess: func(srtPath string) error {
			stats, err := subtitle.CleanFile(srtPath)
			if err == nil && stats.Changed() {
				slog.Info("cleaned subtitles", "path", srtPath, "cuesDropped", stats.CuesDropped, "linesCollapsed", stats.LinesCollapsed, "linesUncensored", stats.LinesUncensored, "musicCuesDropped", stats.MusicCuesDropped, "longCuesDropped", stats.LongCuesDropped, "fillerCuesDropped", stats.FillerCuesDropped, "fillerLinesTrimmed", stats.FillerLinesTrimmed, "soundCuesDropped", stats.SoundCuesDropped, "soundLinesCleaned", stats.SoundLinesCleaned)
			}
			return err
		},
	}
	queue := job.NewQueue(store, pipeline, broker)

	api := httpapi.NewServer(cfg, store, queue, broker)

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	mux.Handle("/", spaHandler(distFS, cfg.APIToken))

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: httpapi.WithMiddleware(httpapi.WithToken(mux, cfg.APIToken)),
		// ReadHeaderTimeout guards against slow-header attacks without
		// touching response write duration. Deliberately no WriteTimeout:
		// it would be a fixed deadline from when headers finish reading,
		// not reset by ongoing writes, and would cut off long-lived SSE
		// connections during a multi-minute transcription. See
		// internal/httpapi/sse_handlers.go.
		ReadHeaderTimeout: 5 * time.Second,
	}
	// All-jobs event streams never end on their own; end them when Shutdown starts.
	srv.RegisterOnShutdown(api.CloseStreams)
	return &App{Server: srv, queue: queue}, nil
}

// requireLoopbackAddr refuses to listen anywhere but a loopback interface:
// the API browses the filesystem and can point the app at arbitrary
// executables, so it must never be reachable from the network.
func requireLoopbackAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("refusing to listen on %q: only loopback addresses (127.0.0.1, ::1, localhost) are allowed", addr)
}

// spaHandler serves the embedded frontend build, falling back to
// index.html for any path that isn't a real file in distFS. The current UI
// is a single page with no client-side router, so the fallback isn't
// exercised today, but it's the standard/expected way to host a Vite SPA
// and costs nothing to have in place for when that changes.
//
// index.html is served with tokenPlaceholder replaced by the per-process API
// token (see config.Config.APIToken); a cross-origin page can't read this
// response, so only the app's own UI learns the token.
func spaHandler(distFS fs.FS, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if upath == "" || upath == "." {
			upath = "index.html"
		}
		if _, err := fs.Stat(distFS, upath); err != nil {
			// A missing asset (anything with a file extension) is a real 404;
			// only extension-less paths are SPA routes that get index.html.
			if path.Ext(upath) != "" {
				http.NotFound(w, r)
				return
			}
			upath = "index.html"
		}
		if upath == "index.html" {
			serveIndex(w, distFS, token)
			return
		}
		// Vite emits content-hashed files under assets/, which never change.
		if strings.HasPrefix(upath, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFileFS(w, r, distFS, upath)
	})
}

// tokenPlaceholder is the literal in web/index.html's api-token <meta> that
// serveIndex swaps for the real token.
const tokenPlaceholder = "__API_TOKEN__"

func serveIndex(w http.ResponseWriter, distFS fs.FS, token string) {
	data, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	data = bytes.ReplaceAll(data, []byte(tokenPlaceholder), []byte(token))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Per-process token: never let a cached copy outlive a restart.
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}
