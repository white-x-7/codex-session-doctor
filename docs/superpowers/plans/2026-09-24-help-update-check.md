# Help and Update Check Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `codex-session-doctor` 增加清晰可发现的帮助命令和安全、可测试的版本更新检查命令。

**Architecture:** 保留现有 CLI 分发层，在 `internal/cli` 增加顶层/子命令帮助路由；在独立的 `internal/update` 包中通过 GitHub 官方 API 查询最新 release，使用标准库 HTTP 客户端、超时和 SemVer 比较，不自动下载或修改本地文件。CLI 只负责参数、中文输出和退出码，网络与版本解析由包内纯函数负责。

**Tech Stack:** Go 1.25、标准库 `flag`/`net/http`/`encoding/json`/`context`/`testing`，现有 `make check` 流程。

**Spec:** 用户需求：“增加 help 指令（查看目前有什么指令）和检查更新功能；新建分支写入，等待审查后合并”。

## Global Constraints

- 不恢复 API 切换、第三方供应商配置或自动升级功能。
- 更新检查只读联网，不下载、覆盖或执行任何文件。
- 网络请求必须有有限超时；只接受 HTTPS 的固定 GitHub API 地址。
- 所有用户可见文案使用中文；保持现有退出码约定。
- 修复命令原有行为和备份安全性不得回退。

## Review Focus

- `help`、`help repair`、`repair --help` 和未知帮助主题是否都给出可操作提示且返回正确退出码。
- GitHub 返回 `v` 前缀、无 `v` 前缀、预发布或非法版本时，比较结果和错误是否稳定。
- 网络超时、非 2xx、无 release（404）时是否快速失败并解释原因，不误报“已是最新”。
- 更新检查是否严格只读，并且测试不会访问真实网络。
- 旧的 `doctor`、`repair`、`version` 命令和跨目录安装使用是否保持兼容。

### Task 1: CLI 帮助命令

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/cli_test.go`
- Modify: `README.md`

**Interfaces:**
- `Run(Env)` 支持 `help [主题]`、`--help`，并让 `repair --help` 输出 repair 专用说明。
- 未知主题返回 `ExitUsage`，不进入修复逻辑。

- [ ] **Step 1: 写失败测试**：覆盖顶层帮助、`help repair`、`repair --help`、未知主题和帮助输出中包含新命令。
- [ ] **Step 2: 运行 CLI 测试确认失败**：`go test ./internal/cli -run 'TestHelp' -count=1`。
- [ ] **Step 3: 实现帮助路由**：拆分通用帮助与 repair 专用帮助，`help` 接受一个可选主题，`--help` 作为每个命令的早期退出开关。
- [ ] **Step 4: 运行测试确认通过**：`go test ./internal/cli -run 'TestHelp' -count=1`。
- [ ] **Step 5: 更新 README 用法并提交**：说明 `codex-session-doctor help`、`help repair` 和 `repair --help`。

### Task 2: 更新检查核心包

**Files:**
- Create: `internal/update/check.go`
- Create: `internal/update/check_test.go`

**Interfaces:**
- `Check(ctx context.Context, client *http.Client, current string) (Result, error)` 查询固定的 GitHub release/tag API，不接受用户提供的远程地址。
- `Result` 提供 `Current`, `Latest`, `URL`, `UpdateAvailable`。
- `Compare(current, latest string) (int, error)` 仅比较 `major.minor.patch`，允许可选 `v` 前缀，拒绝非法版本。

- [ ] **Step 1: 写 HTTP server 与版本比较失败测试**：覆盖新版本、已是最新、旧版本、非法版本、非 2xx 和超时。
- [ ] **Step 2: 运行测试确认失败**：`go test ./internal/update -count=1`。
- [ ] **Step 3: 用标准库实现**：请求设置 `Accept` 与 `User-Agent`，使用 10 秒默认超时，限制响应体大小；优先解析 latest release，无 release 时回退到 tags；不执行下载。
- [ ] **Step 4: 运行测试确认通过**：`go test ./internal/update -count=1`。

### Task 3: 接入 `check-update` 命令

**Files:**
- Create: `internal/cli/update.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/cli_test.go`
- Modify: `README.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- 新命令：`codex-session-doctor check-update`；只读访问 GitHub，发现新版本时打印版本与 release 链接。
- 支持 `--timeout <秒>`，默认 10 秒；非法或非正数返回 `ExitUsage`。
- 网络或响应错误返回 `ExitFailure`，不会改变文件。

- [ ] **Step 1: 写 CLI 注入 HTTP client 的失败测试**：验证新版本、已最新、错误退出码及帮助文字。
- [ ] **Step 2: 实现 CLI 调用**：通过可注入的检查函数/客户端保持单元测试离线，输出中文结果。
- [ ] **Step 3: 运行全量检查**：`make check`、`CGO_ENABLED=0 go build ./cmd/codex-session-doctor`。
- [ ] **Step 4: 更新 README/CHANGELOG**：补充命令、网络行为、隐私说明和示例。
- [ ] **Step 5: 提交功能分支**：`git add ... && git commit -m "feat: add help and update check commands"`。

### Task 4: 审查、合并与清理

**Files:**
- Review the complete branch diff against `main`.

- [ ] **Step 1: 固定审查基点并运行 Standards/Spec 双轴审查**。
- [ ] **Step 2: 修复审查发现并重新运行 `make check`；如有修复，追加提交。**
- [ ] **Step 3: 将功能分支合并到 `main`，推送主分支。**
- [ ] **Step 4: 删除本地和远程功能分支，确认工作区干净。**
