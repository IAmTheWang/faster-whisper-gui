# internal/sseutil

One file, `broker.go`, with no dependency on anything else in this repo. `Broker` is a minimal per-job pub-sub: each job ID is its own topic, and any number of HTTP handlers can `Subscribe` to receive that job's `Event`s (`Type` becomes the SSE `event:` field, `Data` is JSON-marshaled for `data:` by the handler) as `internal/httpapi`'s SSE handler forwards them to the client.

- `Subscribe(jobID)` returns a 32-buffered channel plus an `unsubscribe` func — **callers must call `unsubscribe`** (typically via `defer`) when done reading, or the channel and its map entry leak forever.
- `Publish` is **non-blocking/best-effort** (`select` with a `default`): a subscriber that hasn't drained its channel just misses the event rather than blocking the job pipeline. This means SSE progress is inherently lossy under backpressure — `GET /api/jobs/{id}` (via `internal/job.Store`) remains the source of truth for a job's current state if an SSE client needs to catch up, and `httpapi`'s SSE handler leans on this by sending the job's current state immediately on every new subscription rather than assuming the client saw everything.

`Broker` implements `internal/job`'s `EventPublisher` structurally (duck-typed) — `sseutil` doesn't import `job`, and `job` doesn't import `sseutil`. Keep it that way if you extend either package; the interface lives in `job` specifically so the queue package doesn't need to know anything about SSE as a transport.
