# cmd/dev

Developer convenience only — not what end users run (that's `cmd/server`). `go run ./cmd/dev`, executed from the repo root, starts both halves of the stack from one command:

- The Go backend, built via the same `appserver.New(cfg)` as `cmd/server`, run via `srv.ListenAndServe()` inside a goroutine so it can be `Shutdown` gracefully later.
- A `bun run dev` (Vite) subprocess, started through `internal/procutil.Start` (Job-Object-managed, not a bare `exec.Cmd`) with `Stdout`/`Stderr` wired directly to the parent's — logs from both processes interleave in one terminal, prefixed `[go]`/`[vite]`.

Shutdown is triggered by `signal.NotifyContext(context.Background(), os.Interrupt)` (Ctrl+C): `srv.Shutdown(ctx)` for the HTTP server, then `vite.Stop()` + `vite.Wait()` for the subprocess — `Stop()` calls `TerminateJobObject`, which kills the whole tree bun/vite spawned (confirmed by hand: killing the compiled `dev.exe` outright, simulating a crash, still takes the entire `bun.exe`→`node.exe` chain down via the Job Object's kill-on-close semantics, without this shutdown code ever running).

**Two things worth knowing if you touch this file:**

1. `-root` defaults to `os.Getwd()`, *not* the executable's directory (unlike `cmd/server`). This is deliberate: `go run` compiles to a temp path, so the `cmd/server` convention of resolving relative to the executable would silently break here. This does mean `cmd/dev` must be launched from the repo root (or with an explicit `-root`) for `web/` to be found.
2. If `-addr` overrides the backend port, `FASTER_WHISPER_GUI_BACKEND_ADDR` is set in the spawned bun process's environment so `web/vite.config.ts`'s dev-proxy target follows automatically — without this, Vite would keep proxying `/api` to the hardcoded default and every request would 502. If you add more config that needs to flow from Go to the Vite dev server, extend this same env-var pattern rather than hand-editing `vite.config.ts` per environment.

This file spawns `bun run dev`, not the bare `bun dev` shorthand — the bare form would actually work fine here (`dev` isn't one of bun's own subcommand names), but `run` is used consistently everywhere in this repo since the sibling `build` script *does* collide with bun's built-in bundler (see `web/CLAUDE.md`), and it's simpler to never rely on the shorthand being safe than to remember which script names happen to be exceptions.
