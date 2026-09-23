# 上游来源与本次重构

本仓库不是从零开始的项目，它复刻自一个上游开源仓库，并在此基础上做了重构。

## 上游信息

| 项目 | 内容 |
| --- | --- |
| 上游仓库 | [boommilk1996-milk/codex-api-switch](https://github.com/boommilk1996-milk/codex-api-switch) |
| 上游定位 | Codex API 服务商切换工具（OpenAI ↔ DeepSeek），附带会话历史标签同步与修复 |
| 导入的上游提交 | `dd45014b6ee1272167aaed302131325f4048c0f8` |
| 导入日期 | 2026-09-23 |
| 上游原始说明 | 完整保留在 [docs/upstream-README.md](docs/upstream-README.md)，未做改写 |

上游项目本身也明确声明是非 OpenAI 官方工具。本仓库沿用这一声明。

## 为什么只有上游说明保留英文

`docs/upstream-README.md` 是上游文档的原件，用于标明来源和对照原始描述。它是历史凭证，改写会失去核对价值，因此保持原样。除此之外，本仓库的 README、本文档、变更记录和 `docs/` 下的说明全部使用中文。

## 本次重构做了什么

### 语言与结构

- 实现语言从 Python 重写为 Go，模块路径为 `github.com/white-x-7/codex-session-doctor`
- 拆分出独立的包：`internal/codex`（目录与进程检测）、`internal/rollout`（rollout 解析与重写）、`internal/history`（历史投影缓存）、`internal/backup`（快照）、`internal/repair`（编排）、`internal/cli`（命令行）
- 提供 `Makefile` 与重写后的 `install.sh`，构建产物是单个静态二进制

### 功能裁剪

只保留会话修复能力，移除了与"切换服务商"有关的一切：

- 删除 `deepseek`、`openai`、`sync`、`key` 四个子命令
- 删除 `config.toml` 渲染与恢复点、模型目录写入、API Key 存取、模型缓存清理
- 删除会话级模型与 provider 标签重写
- 删除 macOS JXA 应用、PowerShell 菜单与 `.cmd` 启动器

### 修复能力增强

这些是在上游 `repair` 基础上的加固，也是本仓库存在的主要理由：

- **保持字节长度。** 重写后不足原长时补空格，确保 `thread_history_*.sqlite` 里缓存的字节偏移继续有效，避免 `cutoff byte offset is past the source rollout`
- **清理非官方密文。** 移除无法被官方校验的 `encrypted_content`，同时保留 `gAAAAA` 开头的官方密文
- **多 rollout 一起修。** 一个会话的初始文件与每次续写的分段文件共享会话 id，全部一起处理
- **刷新历史投影。** 按父 id 与每个分段 id 清理 `thread_history_*.sqlite` 中的缓存行，让 Codex 重新解析
- **扫描范围更完整。** 同时扫描 `sessions/` 与 `archived_sessions/`
- **备份更完整。** 历史数据库连同 `-wal`、`-shm` 边车文件一起进快照
- **数字不被浮点化。** 重写时按原始字面量输出数字，避免大整数精度丢失

### 命名

重构后工具不再切换服务商，`codex-api-switch` 这个名字会产生误导，因此改名为 `codex-session-doctor`：它诊断会话历史的问题（`doctor`），并修复无法续聊的会话（`repair`）。

## 与上游的授权关系

上游仓库未附许可证文件。本仓库为私有复刻，仅用于个人使用；如需公开发布或再分发，请先与上游作者确认授权。
