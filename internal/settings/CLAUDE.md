# internal/settings

One file, `settings.go`. `Store` persists user-chosen overrides (`Paths`: `FfmpegPath`, `WhisperCliPath`, `ModelsDir`, `DefaultVideoDir`) to `<root>/settings.json`, letting the web UI point at binaries/models that live outside the conventional `bin/whisper`, `bin/ffmpeg`, `models` layout `internal/config` resolves by default. An empty `Paths` field means "no override, use the default" — this package has no concept of the conventional defaults themselves, that's `internal/config`'s job via its `Effective*` methods. `DefaultVideoDir` is the exception: it has no conventional default to fall back to (see `internal/config`'s `EffectiveDefaultVideoDir`), it's purely "which directory should the video picker open to" and defaults to unset (picker lists all drives).

`Load` treats a missing file as "nothing saved yet" (zero-value `Paths`, no error) but a file that exists and fails to parse *is* an error — a corrupted `settings.json` should surface at startup, not silently discard whatever the user last saved.

`Set` writes atomically: marshal to a temp file in the same directory (`os.CreateTemp`), then `os.Rename` it over `settings.json` (atomic on the same volume), and only updates the in-memory value `Get` returns after the rename succeeds. If anything fails before the rename, the temp file is removed explicitly — a failed save should never leave stray `.settings-*.tmp` files behind, and never touches the previously-saved `settings.json`.

`Store` is safe for concurrent `Get`/`Set` (a `sync.RWMutex`), since `Get` is called on every health check / job creation / model scan while a settings save could happen from a concurrent request.

No dependency on `internal/config` — kept one-directional (`config` depends on this, not the reverse) so `config.Load`'s existing "no dependencies" story only grows by one edge.
