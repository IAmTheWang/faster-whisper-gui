# cmd/server

Production entrypoint. `go build ./cmd/server` yields the single `.exe` end users double-click.

`main.go` does, in order: parse `-root`/`-addr` flags → `config.Load` → `cfg.CleanTmpDir()` → `appserver.New(cfg)` → log the listen address → `srv.ListenAndServe()`. That's the whole file — all actual route/middleware/pipeline wiring lives in [internal/appserver](../../internal/appserver/CLAUDE.md), shared with `cmd/dev`. Don't add logic here beyond flag parsing; put it in `appserver` instead so `cmd/dev` gets it too.

`-root` defaults to `""`, which `config.Load` resolves to the executable's own directory via `os.Executable()` — correct for a double-clicked `server.exe`, but note this is *wrong* under `go run` (which compiles to a temp path). If you need to `go run ./cmd/server` for a quick manual test, pass `-root <repo-root>` explicitly (see how the plan/session history did this during development). `cmd/dev` sidesteps this entirely by defaulting `-root` to the current working directory instead, since it's meant to be run via `go run`.
