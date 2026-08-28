[English](README.md) | 简体中文

# faster-whisper-gui

本机单用户的字幕转录工具：Go 后端 + Vite/TypeScript 前端，用 whisper.cpp 把视频或音频文件转录成 SRT 字幕，默认保存到每个源文件所在目录。

## 准备工作

1. **whisper.cpp（CUDA 版）**：从 [whisper.cpp releases](https://github.com/ggml-org/whisper.cpp/releases) 下载 `whisper-cublas-<cuda版本>-bin-x64.zip`（需要机器已装好匹配版本的 NVIDIA CUDA 驱动），解压后把 `whisper-cli.exe` 和相关 DLL（`ggml*.dll`、`whisper.dll`、`ggml-cuda.dll` 及 CUDA 运行时 DLL）放进 `bin/whisper/`。
2. **ffmpeg**：下载 Windows 版 ffmpeg（含 `ffmpeg.exe` + `ffprobe.exe`），放进 `bin/ffmpeg/`。
3. **模型文件**：下载至少一个 `ggml-*.bin` 模型（如 `ggml-small.bin`），放进 `models/`。

准备完成后目录结构大致是：

```
bin/whisper/whisper-cli.exe
bin/whisper/*.dll
bin/ffmpeg/ffmpeg.exe
bin/ffmpeg/ffprobe.exe
models/ggml-small.bin
```

`bin/`、`models/`、`tmp/` 不会随仓库分发（见 `.gitignore`），首次启动服务器时会自动创建空目录。

如果 ffmpeg、whisper-cli 或模型已经装在别处，不想再复制一份到程序目录下，也可以不放进上述目录，而是打开网页后在 **Environment Settings**（环境设置）面板里分别指定这几项的实际路径（点 "Browse" 按钮在本机磁盘上浏览选择，或直接填写完整路径），还可以设置媒体选择器默认打开的目录。设置会保存下来（可执行文件旁的 `settings.json`），重启程序后依然生效；未设置的项则继续使用上面的默认目录约定。

## 构建

```bash
cd web
bun install
bun run build
cd ..
go build ./cmd/server
```

`bun run build` 会把前端构建产物输出到 `internal/webui/dist/`，Go 通过 `go:embed` 把它编译进最终的单个可执行文件——所以必须先构建前端，再 `go build`。

## 运行

```bash
./server.exe
```

默认监听 `http://127.0.0.1:8080`（只监听本机回环地址，不对外网开放），用浏览器打开即可使用。可选参数：

- `-addr <host:port>`：修改监听地址
- `-root <dir>`：修改 `bin/`、`models/`、`tmp/` 所在的根目录（默认是可执行文件所在目录）

## 使用方法

1. **Select Media（选择媒体文件）**——浏览到一个或多个视频/音频文件，勾选每个想转录的文件；跨文件夹导航时已勾选的项不会丢失。
2. **Transcription Settings（转录设置）**——选一次模型和语言即可，这份配置会应用到本批次所有勾选的文件（不支持逐个文件单独配置）；输出位置可选"与源文件同目录"（默认）或自定义目录。点击 **Start Transcription** 会为每个勾选的文件各建一个任务并排队。
3. **Progress（进度）**——每个任务（不管在跑还是已完成）都有自己的一个标签页，方便同时盯着好几个文件的转录情况；底层其实仍是排队串行执行（见下方"架构简述"），切到后台的标签页也会持续实时更新，不会因为你在看别的标签就停止。任务完成后，点 **Save** 可以把这个任务的 `.srt` 另外复制一份到别的目录，或者点 **Save All Completed** 一键把本批次所有已完成任务的 `.srt` 都复制到同一个目录——这只是多存一份副本，原本保存在源文件旁边（或你指定的输出目录）的那份 `.srt`，在任务完成的那一刻就已经照常写好了，不受这个操作影响。
4. **Job History（任务历史）**——本次运行期间跑过的所有任务，点击任意一条可以重新打开（或跳转到）对应的 Progress 标签页。

## 开发模式

一条命令同时启动 Go 后端和 Vite 前端开发服务器（需要在仓库根目录执行）：

```bash
go run ./cmd/dev
```

日志里 `[go]` 前缀是 Go 后端，`[vite]` 前缀是 Vite；打开 Vite 打印出的地址（通常是 `http://localhost:5173`）即可，它会把 `/api` 请求代理到 Go 后端（见 `web/vite.config.ts`），带 HMR。直接打开 Go 后端自己的端口看到的是上一次 `bun run build` 的嵌入版本，不是实时的前端源码。`Ctrl+C` 会同时优雅关闭两边（Go 用 `http.Server.Shutdown`，Vite 子进程通过 Windows Job Object 连带清理，包括 bun/vite 自己再拉起的子进程）。

`cmd/dev` 支持和 `cmd/server` 一样的 `-addr`/`-root` 参数；如果用 `-addr` 改了 Go 后端端口，Vite 那边的代理目标会通过 `FASTER_WHISPER_GUI_BACKEND_ADDR` 环境变量自动跟着变，不需要手动改 `vite.config.ts`。

也可以还是分两个终端手动跑（`cmd/dev` 本质上就是把下面两条命令合并到一起）：

```bash
# 终端 1
go run ./cmd/server
# 终端 2
cd web && bun run dev
```

## 测试

```bash
go test ./...
```

## 架构简述

- **转录引擎**：whisper.cpp（子进程调用 `whisper-cli.exe`），不是 Python 版 faster-whisper——单机 Go 工具场景下不需要用户装 Python/CUDA 生态。
- **媒体选择**：服务器端文件浏览器（`GET /api/browse`），因为浏览器原生的文件选择/拖拽拿不到本地绝对路径，而"默认保存到源文件同目录"需要这个绝对路径。
- **批量任务，串行执行**：可以一次勾选多个视频/音频文件并共用一套模型/语言/输出配置一起启动，但它们仍会排队、一次只跑一个——单块 GPU 同时跑多个 whisper-cli 进程没有意义。Progress 面板的多标签页只是让你能并排观察多个任务的状态，并不是真的并行执行。
- **进度上报**：SSE（`GET /api/jobs/{id}/events`），主要信号来自 whisper-cli 输出的分段时间戳，辅以其自带的百分比行。
- **Save 是纯新增功能**：任务一完成，`.srt` 依然会照常写到它真正的目标位置，这一点完全没变；"Save"/"Save All Completed"（`POST /api/jobs/{id}/export`）只是按需把一个已完成任务的 `.srt` 再复制一份到另一个位置。
- **进程健壮性**：ffmpeg/whisper-cli 子进程通过 Windows Job Object 托管，取消任务或本程序自身崩溃退出都不会留下孤儿进程。
- 更完整的设计背景见根目录及各子目录下的 `CLAUDE.md`。

## 已知限制

- 任务历史仅存内存，重启后清空。
- 模型/二进制文件需手动下载放置，没有自动下载。
- 仅覆盖 CPU + NVIDIA CUDA；AMD/Intel 显卡的 Vulkan 版本不在范围内。
- 仅支持 Windows。
