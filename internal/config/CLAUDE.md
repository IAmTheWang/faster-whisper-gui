# internal/config

One file, `config.go`. `Config` holds every resolved *absolute* path the rest of the app needs: `WhisperCliPath`, `FfmpegPath`, `FfprobePath`, `ModelsDir`, `TmpDir`, plus `Addr`. Nothing else in the codebase should compute these paths itself — always go through this struct.

`Load(root)` resolves `root` to `filepath.Dir(os.Executable())` when called with `""`, then `os.MkdirAll`s `bin/whisper`, `bin/ffmpeg`, `models`, and `tmp` unconditionally. That `MkdirAll` matters: it means every other package (`fsbrowse`'s directory scans, `transcribe.ScanModels`, the job queue's temp-file writes) can assume these directories exist and never has to special-case `os.ErrNotExist` on a fresh checkout.

`CleanTmpDir()` wipes everything inside `TmpDir` — called once at startup by both `cmd/server` and `cmd/dev` to clear scratch WAV files left behind by a previous run that crashed before `internal/job`'s own per-job cleanup ran.

**Gotcha if you add a new caller of `Load`:** the `""` → `os.Executable()` default is correct for a built `.exe` but resolves to a *temp build directory* under `go run`. `cmd/server` accepts this (it's meant to be built, not `go run`); `cmd/dev` explicitly overrides it to `os.Getwd()` instead because it's meant to be `go run` from the repo root. Pick the right default for whatever you're adding, don't just copy one or the other blindly.
