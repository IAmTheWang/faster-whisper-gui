# web/src

No framework — see [components/CLAUDE.md](components/CLAUDE.md) for the four UI panels. This directory holds the shared plumbing they're built on:

- **`api.ts`** — typed `fetch` wrapper mirroring the Go backend's JSON shapes (`Job`, `Model`, `Listing`, etc. — keep these in sync with the corresponding Go structs in `internal/job`, `internal/transcribe`, `internal/fsbrowse` if the API changes) plus `subscribeJobEvents`, which wraps `EventSource` for a job's SSE stream. **The server's custom `"error"` named SSE event unavoidably shares a name with `EventSource`'s own built-in connection-failure event.** The fix here is checking `typeof (e as MessageEvent).data === 'string'` — a genuine server-sent `"error"` event always carries a JSON string payload, a native transport error doesn't. Don't try to fix this by having the browser ignore the collision differently; if the wire event name ever changes, update it on both sides (see `internal/httpapi/CLAUDE.md`).
- **`state.ts`** — `Store<T>`, a ~20-line pub-sub cell (subscribe gets called immediately with the current value, then again on every `set`). `selectedVideo` and `activeJobId` are the two pieces of state shared across panels; add more here rather than reaching into another component's internals.
- **`dom.ts`** — `escapeHtml`, used wherever a component interpolates a filesystem-derived string (filename, path) into an `innerHTML` template string. Windows filenames can't actually contain `<`/`>`, so this isn't defusing a realistic injection on this platform — it's cheap insurance against `&` breaking entity parsing and against relying on that platform detail at all.
- **`main.ts`** — wires the four panels together plus the health-check banner (`api.health()` on load, shown/hidden based on whether ffmpeg/whisper-cli probed OK).

There's no build step for this directory beyond what Vite/tsc already do — no bundler config, no path aliases, plain relative imports throughout.
