[English](README.md) | 简体中文

# faster-whisper-gui

本机单用户的字幕转录工具：Go 后端 + Vite/TypeScript 前端，用 whisper.cpp 把视频转录成 SRT 字幕，默认保存到视频所在目录。

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

如果 ffmpeg、whisper-cli 或模型已经装在别处，不想再复制一份到程序目录下，也可以不放进上述目录，而是打开网页后在"环境设置"面板里分别指定这三者的实际路径（支持点"浏览"在本机磁盘上选择，或直接填写完整路径）——设置会保存下来，重启程序后依然生效，未设置的项则继续使用上面的默认目录约定。

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
- **视频选择**：服务器端文件浏览器（`GET /api/browse`），因为浏览器原生的文件选择/拖拽拿不到本地绝对路径，而"默认保存到视频同目录"需要这个绝对路径。
- **进度上报**：SSE（`GET /api/jobs/{id}/events`），主要信号来自 whisper-cli 输出的分段时间戳，辅以其自带的百分比行。
- **进程健壮性**：ffmpeg/whisper-cli 子进程通过 Windows Job Object 托管，取消任务或本程序自身崩溃退出都不会留下孤儿进程。
- 更完整的设计背景见仓库内各包的注释，以及本项目开发过程中产出的架构计划文档。

## 已知限制

- 任务历史仅存内存，重启后清空。
- 模型/二进制文件需手动下载放置，没有自动下载。
- 仅覆盖 CPU + NVIDIA CUDA；AMD/Intel 显卡的 Vulkan 版本不在范围内。
- 仅支持 Windows。
