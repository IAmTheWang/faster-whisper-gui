# internal/appserver

One file, `appserver.go`. `New(cfg *config.Config) (*http.Server, error)` is the single place that assembles the whole backend: builds the job `Store`/`Queue`/`sseutil.Broker`, wires the `transcribe.WhisperCppEngine` into a `job.Pipeline` (via closures over `transcribe.ExtractAudio`/`transcribe.ProbeDuration`, not direct dependencies — see `internal/job/CLAUDE.md`), constructs an `httpapi.Server`, and mounts it alongside the embedded frontend (`internal/webui`) with the SPA-fallback static handler.

The `WhisperCppEngine`/pipeline closures are wired to `cfg`'s `Effective*` methods (`EffectiveWhisperCliPath`, `EffectiveFfmpegPath`, `EffectiveFfprobePath`), not the plain `cfg.WhisperCliPath`/`cfg.FfmpegPath`/`cfg.FfprobePath` fields — this is what makes a Settings-panel path override take effect on the next job without restarting the process, since the single shared `engine`/pipeline is constructed once here but each method call re-reads `cfg.Settings` fresh.

It returns the `*http.Server` **without calling `ListenAndServe`** — that's deliberate, so `cmd/dev` can run it in a goroutine and `Shutdown` it later, while `cmd/server` can just call `ListenAndServe` directly. Both entrypoints call this same function; if you need to change routing, middleware, or how the pipeline is wired, do it here so neither entrypoint drifts from the other.

`spaHandler` serves the embedded `dist` filesystem and falls back to `index.html` for any path it can't find — the current UI has no client-side router so this fallback is currently unexercised, but it's the standard way to host a Vite SPA and it's free to have in place.

The `ReadHeaderTimeout`/no-`WriteTimeout` choice on the returned `*http.Server` is explained in `internal/httpapi/CLAUDE.md` (the SSE handler is why).
