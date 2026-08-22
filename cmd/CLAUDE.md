# cmd/

Two entrypoints, both thin wrappers around [internal/appserver](../internal/appserver/CLAUDE.md) so the actual HTTP wiring exists in exactly one place:

- **[server/](server/CLAUDE.md)** — the production build. `go build ./cmd/server` produces the single `.exe` end users run; it embeds the frontend via `internal/webui` and just calls `appserver.New(cfg)` then `ListenAndServe()`.
- **[dev/](dev/CLAUDE.md)** — a developer convenience only, not shipped to end users. Runs the same Go backend (in a goroutine, so it can be gracefully `Shutdown`) alongside a `bun run dev` (Vite) subprocess, so day-to-day frontend work doesn't need two terminals.

If you change how the server is constructed (routes, middleware, config wiring), do it in `internal/appserver`, not in either `main.go` — that's the whole point of the split.
