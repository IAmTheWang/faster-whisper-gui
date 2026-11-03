// Package appserver assembles the fully-wired HTTP handler (API routes +
// embedded frontend, with the SPA fallback and middleware applied) shared
// by cmd/server (the production single-binary build) and cmd/dev (which
// pairs the same Go backend with the Vite dev server), so the two
// entrypoints can't drift apart.
package appserver

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"faster-whisper-gui/internal/config"
	"faster-whisper-gui/internal/httpapi"
	"faster-whisper-gui/internal/job"
	"faster-whisper-gui/internal/sseutil"
	"faster-whisper-gui/internal/transcribe"
	"faster-whisper-gui/internal/webui"
)

// New builds an *http.Server for cfg, ready to ListenAndServe. It does not
// start listening — callers own that, so cmd/dev can run it in a goroutine
// and Shutdown it later alongside the Vite dev server it launches.
func New(cfg *config.Config) (*http.Server, error) {
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
	}
	queue := job.NewQueue(store, pipeline, broker)

	api := httpapi.NewServer(cfg, store, queue, broker)

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	mux.Handle("/", spaHandler(distFS))

	return &http.Server{
		Addr:    cfg.Addr,
		Handler: httpapi.WithMiddleware(mux),
		// ReadHeaderTimeout guards against slow-header attacks without
		// touching response write duration. Deliberately no WriteTimeout:
		// it would be a fixed deadline from when headers finish reading,
		// not reset by ongoing writes, and would cut off long-lived SSE
		// connections during a multi-minute transcription. See
		// internal/httpapi/sse_handlers.go.
		ReadHeaderTimeout: 5 * time.Second,
	}, nil
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
func spaHandler(distFS fs.FS) http.Handler {
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
		http.ServeFileFS(w, r, distFS, upath)
	})
}
