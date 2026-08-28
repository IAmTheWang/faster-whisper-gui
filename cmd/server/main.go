// Command server is the faster-whisper-gui backend: a local-only HTTP
// server that serves the embedded frontend and drives the media -> SRT
// transcription pipeline (see the project plan at
// C:\Users\carlos\.claude\plans\go-ts-faster-whisper-glowing-possum.md for
// the full design). This is the production entrypoint — for iterating on
// the frontend with Vite's dev server/HMR, use cmd/dev instead.
package main

import (
	"flag"
	"log"

	"faster-whisper-gui/internal/appserver"
	"faster-whisper-gui/internal/config"
)

func main() {
	root := flag.String("root", "", "directory containing bin/, models/, tmp/ (defaults to the executable's own directory)")
	addr := flag.String("addr", "", "listen address override (defaults to 127.0.0.1:8080)")
	flag.Parse()

	cfg, err := config.Load(*root)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if *addr != "" {
		cfg.Addr = *addr
	}

	if err := cfg.CleanTmpDir(); err != nil {
		log.Printf("warning: failed to clean tmp dir %s: %v", cfg.TmpDir, err)
	}

	srv, err := appserver.New(cfg)
	if err != nil {
		log.Fatalf("build server: %v", err)
	}

	log.Printf("faster-whisper-gui listening on http://%s", cfg.Addr)
	log.Fatal(srv.ListenAndServe())
}
