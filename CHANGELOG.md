# 变更记录

## 未发布

- 新增 `help [命令]` 和各子命令的 `--help`，可直接查看当前支持的命令与参数。
- 新增 `check-update`，只读检查 GitHub 最新 release/tag，不自动下载或安装。

## 0.1.0 —— 2026-09-23

首个 Go 版本，项目更名并收窄为「会话修复」单一职责。

### 新增

- `repair` 子命令：修复无法在官方接口下续聊的会话，支持指定会话或 `--all`
- `doctor` 子命令：只读体检，报告主目录、进程状态、rollout 数量与待修会话
- `version` 子命令与完整的中文帮助信息
- `Makefile` 与 Go 版 `install.sh`
- 测试覆盖字节长度保持、密文区分、幂等性、多 rollout 会话、历史投影清理、运行中拒绝写入与端到端修复

### 改进

- 重写 rollout 时保持每行字节长度，避免 Codex 分页历史偏移失效
- 改写改为「同目录临时文件 + 改名」的原子写入，避免中途被打断留下残缺文件
- 同时处理同一会话的初始 rollout 与全部续写分段
- 按父会话 id 与分段 id 刷新 `thread_history_*.sqlite` 缓存行
- 备份历史数据库时一并复制 `-wal`、`-shm` 边车文件
- 扫描范围扩展到 `archived_sessions/`
- 重写时保留数字的原始字面量，避免大整数精度丢失
- 历史库备份失败时保留数据库原样，不再冒险修改
- 读不了的 rollout 改为报出警告，不再静默跳过
- 修复中途失败时，错误信息带上本次快照目录，方便直接回滚
- 清理历史投影时不再以创建模式打开数据库，避免凭空生成空的 `thread_history_*.sqlite`
- 检测命令超时改为返回专门的错误，不再复用「命令不存在」

### 移除

- 服务商切换：`deepseek`、`openai` 子命令
- 历史标签与模型同步：`sync` 子命令
- API Key 管理：`key` 子命令
- `config.toml` 渲染、恢复点、模型目录与模型缓存清理
- macOS JXA 应用、PowerShell 菜单、`.cmd` 启动器
- 一键发布脚本 `push-all.sh`
- `status` 与 `is-running` 两个子命令（进程状态并入 `doctor` 输出）
- Python 实现与其测试脚本

### 更名

- 项目与二进制名从 `codex-api-switch` 改为 `codex-session-doctor`
- 备份目录从 `~/.codex/backups/codex-api-switch/` 改为 `~/.codex/backups/codex-session-doctor/`
- 隔离用环境变量从 `CODEX_SWITCH_HOME` 改为 `CODEX_SESSION_DOCTOR_HOME`

## 上游历史

本版本之前的历史来自上游仓库 [boommilk1996-milk/codex-api-switch](https://github.com/boommilk1996-milk/codex-api-switch)，其提交记录保留在本仓库的 Git 历史中。
