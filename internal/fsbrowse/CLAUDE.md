# internal/fsbrowse

Everything about touching the local filesystem on the frontend's behalf lives here. This whole package exists because the tool's core feature (save the SRT next to the source video) requires an absolute local path, and a browser's native file picker/drag-and-drop will never hand one back — so instead the server browses the disk itself and the frontend just renders what it returns.

- `ListDrives()` — enumerates drive letters via `windows.GetLogicalDrives`/`GetDriveType`, skipping `DRIVE_NO_ROOT_DIR`/`DRIVE_UNKNOWN`.
- `List(dir)` — lists `dir`'s subdirectories and video files only (everything else filtered out; see `videoExtensions`/`IsVideoFile`), directories first then alphabetical, case-insensitive. Expects `dir` to already be absolute/cleaned — validate with `ValidateAbsDir` first, this function doesn't re-check.
- `ValidateAbsDir` / `ValidateVideoFile` — the normalization boundary for any path arriving from the client. Both run `filepath.Clean(filepath.FromSlash(p))` before validating, because the frontend's "custom output directory" field is free-typed text and can contain forward slashes. Every HTTP handler that receives a path from the request should go through one of these, not `os.Stat` directly.
- `EnsureWritable(path)` — opens (creating if absent, never truncating existing content) then immediately closes `path`, used to fail a job submission fast if the target `.srt` is locked by something (e.g. a media player has it open). **Treat any non-nil error as "not writable"** — don't pattern-match `os.ErrPermission` specifically. Windows reports a locked file as a sharing violation, which is a different error than access-denied, so a narrow check would silently miss the single most common real-world case this function exists to catch.

Tests use `t.TempDir()` real-filesystem fixtures throughout, no mocking — see `fsbrowse_test.go` for the pattern if you add more path-handling functions here.
