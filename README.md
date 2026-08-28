English | [简体中文](README.zh-CN.md)

# faster-whisper-gui

A local, single-user subtitle transcription tool: Go backend + Vite/TypeScript frontend, using whisper.cpp to transcribe video or audio files into SRT subtitles, saved next to each source file by default.

## Prerequisites

1. **whisper.cpp (CUDA build)**: download `whisper-cublas-<cuda-version>-bin-x64.zip` from the [whisper.cpp releases](https://github.com/ggml-org/whisper.cpp/releases) page (requires a matching NVIDIA CUDA driver already installed), then unzip `whisper-cli.exe` and its DLLs (`ggml*.dll`, `whisper.dll`, `ggml-cuda.dll`, and the CUDA runtime DLLs) into `bin/whisper/`.
2. **ffmpeg**: download a Windows build of ffmpeg (including `ffmpeg.exe` + `ffprobe.exe`) into `bin/ffmpeg/`.
3. **Model files**: download at least one `ggml-*.bin` model (e.g. `ggml-small.bin`) into `models/`.

Once done, the layout looks roughly like:

```
bin/whisper/whisper-cli.exe
bin/whisper/*.dll
bin/ffmpeg/ffmpeg.exe
bin/ffmpeg/ffprobe.exe
models/ggml-small.bin
```

`bin/`, `models/`, and `tmp/` are not shipped with the repo (see `.gitignore`) — they're created automatically as empty directories the first time the server starts.

If ffmpeg, whisper-cli, or your models already live elsewhere and you'd rather not copy them into this program's own folder, you don't have to use the layout above at all — open the web UI and set each path individually in the **Environment Settings** panel (browse the local disk with the "Browse" button, or type a full path directly). You can also set a default directory to open the media picker in. Saved overrides persist across restarts (in `settings.json` next to the executable); anything left unset keeps using the conventional directory above as its default.

## Build

```bash
cd web
bun install
bun run build
cd ..
go build ./cmd/server
```

`bun run build` outputs the frontend build to `internal/webui/dist/`, which Go embeds into the final single executable via `go:embed` — so the frontend must be built before `go build`.

## Run

```bash
./server.exe
```

Listens on `http://127.0.0.1:8080` by default (loopback only, never exposed to the network) — open it in a browser to use the tool. Optional flags:

- `-addr <host:port>`: override the listen address
- `-root <dir>`: override the directory containing `bin/`, `models/`, `tmp/` (defaults to the executable's own directory)

## Usage

1. **Select Media** — browse to one or more video or audio files and check the box next to each one you want transcribed. Selections persist as you navigate between folders.
2. **Transcription Settings** — pick a model and language once; this configuration is shared by every checked file in the batch (there's no per-file config). Choose the default "same directory as the source file" output, or a custom output directory. Click **Start Transcription** to queue a job for each checked file.
3. **Progress** — each running or finished job gets its own tab, so you can watch several files' transcriptions at a glance. Jobs still process one at a time under the hood (see [Architecture](#architecture-at-a-glance)); background tabs keep updating live even while you're looking at a different one. Once a job is done, use **Save** to copy that job's `.srt` to another folder, or **Save All Completed** to copy every finished job's `.srt` in the batch to one folder in one click — this is purely an extra copy, the original `.srt` next to the source file (or in your chosen output directory) is written the moment the job finishes either way.
4. **Job History** — every job ever run this session, click one to reopen (or jump back to) its Progress tab.

## Development

Start both the Go backend and the Vite frontend dev server with a single command (run from the repository root):

```bash
go run ./cmd/dev
```

Log lines are prefixed `[go]` for the backend and `[vite]` for the frontend. Open the URL Vite prints (usually `http://localhost:5173`) — it proxies `/api` requests to the Go backend (see `web/vite.config.ts`) and gives you HMR. Hitting the Go backend's own port directly serves whatever was embedded by the last `bun run build`, not your live frontend source. `Ctrl+C` shuts both down gracefully (`http.Server.Shutdown` for Go; the Vite subprocess tree — including any children bun/vite spawn — is cleaned up via a Windows Job Object).

`cmd/dev` accepts the same `-addr`/`-root` flags as `cmd/server`. If you override the backend port with `-addr`, Vite's proxy target follows automatically via the `FASTER_WHISPER_GUI_BACKEND_ADDR` environment variable — no need to edit `vite.config.ts` by hand.

You can still run the two halves manually in separate terminals if you prefer (`cmd/dev` is just these two commands wired together):

```bash
# terminal 1
go run ./cmd/server
# terminal 2
cd web && bun run dev
```

## Testing

```bash
go test ./...
```

## Architecture at a glance

- **Transcription engine**: whisper.cpp, invoked as a subprocess (`whisper-cli.exe`) rather than Python's faster-whisper — a local single-user Go tool doesn't need a Python/CUDA userland to install.
- **Media selection**: a server-side file browser (`GET /api/browse`), because a browser's native file picker/drag-and-drop can't hand back an absolute local path, and saving next to the source file by default requires that absolute path.
- **Batch jobs, serial execution**: you can check and start several video/audio files at once, sharing one model/language/output config, but they're queued and run one at a time — a single GPU can't usefully run concurrent whisper-cli processes. The Progress panel's tabs just let you monitor several jobs' states side by side, not run them in parallel.
- **Progress reporting**: SSE (`GET /api/jobs/{id}/events`), primarily derived from whisper-cli's own per-segment timestamps, supplemented by its built-in percentage output.
- **Save is additive**: the `.srt` is always written to its real destination the moment a job finishes, exactly as before; "Save"/"Save All Completed" (`POST /api/jobs/{id}/export`) just copies an already-finished job's `.srt` to a second location on demand.
- **Process robustness**: ffmpeg/whisper-cli subprocesses are managed through a Windows Job Object, so canceling a job or the server itself crashing never leaves an orphaned process behind.
- See `CLAUDE.md` (and the `CLAUDE.md` in each package/directory) for the full design rationale behind each of these.

## Known limitations

- Job history is in-memory only and is lost on restart.
- Model/binary files must be downloaded and placed manually — no auto-download.
- Only CPU + NVIDIA CUDA are covered; AMD/Intel Vulkan builds are out of scope.
- Windows only.
