# internal/

All real logic lives here; `cmd/` is just entrypoints. Roughly bottom-up dependency order (lower items depend on higher ones):

1. **[procutil](procutil/CLAUDE.md)** — Windows subprocess plumbing (Job Object lifecycle, `\r`/`\n`-aware line scanning). No dependencies on anything else here; everything else that spawns a process depends on this.
2. **[config](config/CLAUDE.md)** — resolves the on-disk layout (`bin/whisper`, `bin/ffmpeg`, `models/`, `tmp/`) into absolute paths. No dependencies.
3. **[transcribe](transcribe/CLAUDE.md)** — the video→wav→srt pipeline pieces (ffmpeg invocation, the `Engine` interface, `WhisperCppEngine`, health checks, model scanning). Depends on `procutil`.
4. **[fsbrowse](fsbrowse/CLAUDE.md)** — exposes the local filesystem to the frontend (drive/directory listing, path validation/normalization). No dependencies on the above.
5. **[sseutil](sseutil/CLAUDE.md)** — a minimal per-job SSE pub-sub broker. No dependencies; `job` talks to it only through an interface `job` itself defines, to avoid a cycle.
6. **[job](job/CLAUDE.md)** — the job queue/state machine that drives `transcribe` and reports through something shaped like `sseutil.Broker`.
7. **[httpapi](httpapi/CLAUDE.md)** — the REST/SSE handlers, wiring together `job`, `fsbrowse`, `transcribe` (for model listing), and `config`.
8. **[webui](webui/CLAUDE.md)** — `go:embed`s the built frontend. Independent of everything else; exists only because of where Go resolves embed paths from.
9. **appserver** (one file, `appserver.go`, no subfolder) — the final assembly: builds the `*http.Server` from `httpapi` + `webui` + `job`/`sseutil`/`transcribe`/`config`. This is what both `cmd/server` and `cmd/dev` call.

Everything here targets Windows only — `fsbrowse` and `procutil` call `golang.org/x/sys/windows` directly, and `job`'s single-worker queue assumes a Job Object exists for cleanup, not POSIX process groups.
