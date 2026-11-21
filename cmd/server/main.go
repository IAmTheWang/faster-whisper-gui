// Command server is the faster-whisper-gui backend: a local-only HTTP
// server that serves the embedded frontend and drives the media -> SRT
// transcription pipeline (see the project plan at
// C:\Users\carlos\.claude\plans\go-ts-faster-whisper-glowing-possum.md for
// the full design). This is the production entrypoint — for iterating on
// the frontend with Vite's dev server/HMR, use cmd/dev instead.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"faster-whisper-gui/internal/appserver"
	"faster-whisper-gui/internal/config"
)

// version is stamped at build time (scripts/build.ps1 and CI pass
// -ldflags "-X main.version=..."); a plain `go build` reports "dev".
var version = "dev"

func main() {
	root := flag.String("root", "", "directory containing bin/, models/, tmp/ (defaults to the executable's own directory)")
	addr := flag.String("addr", "", "listen address override (defaults to 127.0.0.1:8080)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("faster-whisper-gui", version)
		return
	}

	cfg, err := config.Load(*root)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if *addr != "" {
		cfg.Addr = *addr
	}

	srv, err := appserver.New(cfg)
	if err != nil {
		log.Fatalf("build server: %v", err)
	}

	// Bind before touching tmp/: a second instance that fails to bind must
	// not wipe the scratch WAVs of the instance that is already running.
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Fatalf("listen on %s: %v", cfg.Addr, err)
	}
	if err := cfg.CleanTmpDir(); err != nil {
		log.Printf("warning: failed to clean tmp dir %s: %v", cfg.TmpDir, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	log.Printf("faster-whisper-gui %s listening on http://%s", version, cfg.Addr)

	select {
	case err := <-serveErr:
		log.Fatalf("server: %v", err)
	case <-ctx.Done():
	}

	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Open SSE streams never end on their own, so Shutdown may time out;
	// Close then drops them. Running whisper/ffmpeg children are killed by
	// their Job Objects when this process exits.
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		_ = srv.Close()
	}
}
