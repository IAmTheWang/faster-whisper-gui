# Go 版 whisper.cpp 调用 vs. Python 版 whisper 调用

这个项目（Go + whisper-cli.exe 子进程）和 Python 生态里的 `openai-whisper`/`faster-whisper`，本质上跑的都是**同一个训练好的 Whisper 模型**，但"语言老板"调度"C++ 工人"干活的方式不一样。

## 核心比喻

同一本武功秘籍（Whisper 模型架构 + 权重），被三家公司（whisper.cpp、PyTorch、CTranslate2）各自照着重新训练出一个 C++ 工人。招式大体一样，但不是同一个人——量化精度、解码循环里的小习惯（比如要不要一直记着前面说过的话）各家不完全一样，所以干出来的活"像但不完全一样"。

真正决定性的区别，是**老板和工人在不在同一个办公室**：

- **Python 这边（`openai-whisper` 用 PyTorch，`faster-whisper` 用 CTranslate2）**：工人就在老板隔壁工位，老板喊一声，工人直接干，结果当场递过来——这叫**同进程调用**（Python 通过原生扩展/绑定，比如 pybind11，直接调用编译好的 C++ 函数，跟 Python 解释器在同一个操作系统进程里）。
- **这个 Go 项目**：老板压根不跟工人在一栋楼里，是**外包**给另一家独立公司（`whisper-cli.exe`），写一张工单（命令行参数：`-m 模型路径 -f 音频路径 -l 语言 -osrt -mc 0` 等）派过去，工人干完活自己把文件送到指定地址（直接把 `.srt` 写到磁盘），中间靠传纸条（stdout/stderr）汇报进度——这叫**子进程调用**（`os/exec.CommandContext` 启动一个独立的操作系统进程）。这也是为什么 `whisper-cli` 就算干活干崩溃了，Go 老板这边一点事没有：不在一个楼里，塌了也砸不到。

## 对照表

| | Python (`openai-whisper` / `faster-whisper`) | 这个项目（Go） |
|---|---|---|
| 干活的工人 | PyTorch / CTranslate2（C++/CUDA） | whisper.cpp（`whisper-cli.exe`，C++） |
| 调用方式 | 原生扩展绑定，同进程内函数调用 | 子进程（`os/exec`），命令行参数 + stdout/stderr |
| 崩溃隔离 | 工人崩了，老板（Python 进程）一起崩 | 工人（`whisper-cli.exe`）崩了，Go 主进程不受影响 |
| 部署要求 | 需要装 Python/PyTorch/CUDA 运行环境 | 只需要一个编译好的 `.exe`，不需要 Python/CUDA 环境 |
| 字幕文件谁写 | Python 代码自己拿到结果后写文件 | `whisper-cli.exe` 自己直接写 `.srt`，Go 不解析字幕内容 |

## 为什么这个项目选子进程而不是绑定

见根目录 [CLAUDE.md](../CLAUDE.md) 和 [internal/transcribe/CLAUDE.md](../internal/transcribe/CLAUDE.md)：不需要在目标机器上装 C/C++ 工具链就能 `go build` 这个仓库；whisper-cli 崩溃不会把 Go 主进程一起带崩；换 CPU 版/CUDA 版 whisper.cpp，只需要换 `bin/whisper/` 目录下的可执行文件，不需要重新编译 Go 代码。
