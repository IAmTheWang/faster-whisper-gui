// Command dev is a developer convenience: it starts the Go API server and
// the Vite frontend dev server together from a single command, so day-to-day
// frontend/backend iteration doesn't need two terminals. It is not what end
// users run — see cmd/server for the production single-binary build.
//
// Point your browser at whatever URL Vite prints below (normally
// http://localhost:5173) — that dev server proxies /api to the Go backend
// (see web/vite.config.ts) and gives you HMR. Hitting the Go backend's own
// port directly serves the last embedded (bun run build) frontend, not your
// live source.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"time"

	"faster-whisper-gui/internal/appserver"
	"faster-whisper-gui/internal/config"
	"faster-whisper-gui/internal/procutil"
)

func main() {
	root := flag.String("root", "", "directory containing bin/, models/, tmp/, and web/ (defaults to the current working directory — run this from the repo root)")
	addr := flag.String("addr", "", "Go backend listen address override (defaults to 127.0.0.1:8080)")
	flag.Parse()

	rootDir := *root
	if rootDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("get working directory: %v", err)
		}
		rootDir = cwd
	}

	cfg, err := config.Load(rootDir)
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		log.Printf("[go]   backend listening on http://%s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[go]   server error: %v", err)
		}
	}()

	webDir := filepath.Join(rootDir, "web")
	viteCmd := exec.CommandContext(ctx, "bun", "run", "dev")
	viteCmd.Dir = webDir
	viteCmd.Stdout = os.Stdout
	viteCmd.Stderr = os.Stderr
	viteCmd.Env = append(os.Environ(), "FASTER_WHISPER_GUI_BACKEND_ADDR="+cfg.Addr)

	vite, err := procutil.Start(viteCmd)
	if err != nil {
		log.Fatalf("start vite dev server (in %s): %v", webDir, err)
	}
	defer vite.Close()

	log.Printf("[vite] starting frontend dev server in %s ...", webDir)
	log.Println("open the URL Vite prints below (usually http://localhost:5173) — it proxies /api to the Go backend above")

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[go]   shutdown error: %v", err)
	}

	if err := vite.Stop(); err != nil {
		log.Printf("[vite] stop error: %v", err)
	}
	_ = vite.Wait()
}
