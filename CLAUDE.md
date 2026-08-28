# faster-whisper-gui

A local, single-user Windows tool: Go backend + Vite/TypeScript frontend that transcribes video or audio files into SRT subtitle files using whisper.cpp, saved next to each source file by default. See [README.md](README.md) / [README.zh-CN.md](README.zh-CN.md) for user-facing setup/usage docs — this file is oriented at whoever (human or AI) is editing the code.

## Architecture at a glance

- **Transcription engine**: whisper.cpp, invoked as a subprocess (`whisper-cli.exe`), not Python's faster-whisper or a cgo binding. See [internal/transcribe/CLAUDE.md](internal/transcribe/CLAUDE.md) and [internal/procutil/CLAUDE.md](internal/procutil/CLAUDE.md) for why (no Python/CUDA userland to install; no C/C++ toolchain needed to build this repo; a whisper-cli crash can't take the Go process down with it).
- **Media selection**: a server-side file browser (`internal/fsbrowse`), because a browser's native file picker/drag-and-drop can't hand back an absolute local path, and "save next to the source file" needs that absolute path. The frontend lets several video/audio files be checked at once and submits one job per file, sharing a single model/language/output config.
- **Batch jobs, serial execution**: `internal/job.Queue` runs exactly one job at a time regardless of how many were submitted together — a single GPU can't usefully run concurrent whisper-cli processes. The frontend's per-job Progress tabs only visualize several jobs' states side by side; they don't parallelize execution.
- **Progress reporting**: SSE, primarily derived from whisper-cli's own per-segment stdout timestamps divided by the audio duration, not its own (historically buggy) stderr percentage line.
- **Save is additive, not a rewrite**: the `.srt` is written to its real destination the moment a job finishes, exactly as it always has been. `POST /api/jobs/{id}/export` (`internal/fsbrowse.CopyFile`) only copies an already-finished job's `.srt` to a second, user-chosen folder on demand — it never changes where the original was written.
- **User-configurable paths**: `internal/settings` persists optional overrides (ffmpeg/whisper-cli paths, models directory, default media directory) as `<root>/settings.json`; `internal/config`'s `Effective*` methods layer those overrides on top of the `bin/`/`models/` conventional defaults, so a Settings-panel change takes effect on the next job with no restart.
- **Process robustness**: every subprocess (ffmpeg, whisper-cli, and in `cmd/dev`, bun/vite) is bound to a Windows Job Object, so canceling a job — or this program crashing — can't leave an orphaned process behind. This is the single most load-bearing piece of Windows-specific plumbing in the codebase; see [internal/procutil/CLAUDE.md](internal/procutil/CLAUDE.md) before touching any subprocess code.
- **Packaging**: the built frontend is embedded into the Go binary via `go:embed` (see [internal/webui/CLAUDE.md](internal/webui/CLAUDE.md)) — the shipped artifact is a single `.exe`.
- **Windows only.** Nothing here has been made cross-platform, and several packages (`fsbrowse`, `procutil`) use Windows-specific APIs directly.

## Directory map

- [cmd/](cmd/CLAUDE.md) — the two entrypoints (`server` production build, `dev` convenience wrapper)
- [internal/](internal/CLAUDE.md) — all real logic, package-by-package
- [web/](web/CLAUDE.md) — the Vite/TypeScript frontend source (build output lives in `internal/webui/dist/`, not `web/dist/`)
- `bin/`, `models/`, `tmp/` — empty, gitignored, user/runtime-managed directories (whisper-cli/ffmpeg binaries, `.bin` models, scratch WAV files); see the README, not worth a CLAUDE.md
- `.serena/` — Serena MCP tool config, unrelated to this project's own code

## Commands

```bash
go build ./cmd/server        # production build (frontend must already be built into internal/webui/dist/)
go run ./cmd/dev             # one command: Go backend + Vite dev server together, run from repo root
go test ./...
cd web && bun install && bun run build   # frontend; always `bun run <script>`, never the bare `bun build`/`bun dev` shorthand — bun has built-in subcommands with those exact names that would shadow the package.json script
```

## Cross-cutting conventions worth knowing before editing

- The server binds `127.0.0.1` only, never `0.0.0.0` — the file-browser API exposes real filesystem paths and must never be reachable off-box.
- Any path arriving from the frontend (a hand-typed custom output directory, a Settings-panel override, in particular) gets `filepath.Clean(filepath.FromSlash(p))` before use — see `internal/fsbrowse.ValidateAbsDir`/`ValidateMediaFile`/`ValidateExeFile` and `internal/job.OutputPrefix`.
- Nothing sets `http.Server.WriteTimeout` — deliberately. It's a fixed deadline from when headers finish reading, not reset by ongoing writes, and would kill a long SSE connection during a multi-minute transcription. `ReadHeaderTimeout` is set instead (safe: doesn't touch response duration).
- `internal/job`'s `Store.Get` returns a value copy of `Job`, never the live pointer — the worker goroutine mutates jobs in place under a lock, so handing out the pointer would race any caller reading it afterward.
- `PUT /api/settings`'s request fields are all `*string`, not `string` — this is how the handler tells "field absent from the JSON body" (leave that override untouched) apart from "field present as `\"\"`" (explicitly clear it back to the conventional default). If you add a new user-configurable path, follow this pattern rather than a plain string with `omitempty`.
- Package manager is **bun**, not npm (see `web/CLAUDE.md` and the root `.gitignore`, which ignores a stray `package-lock.json` in case npm gets run by mistake).
