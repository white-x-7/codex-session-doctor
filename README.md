# codex-session-doctor

诊断并修复 Codex 的本地会话历史，让被第三方接口推进过的会话能够重新在官方接口下显示和续聊。

> 本仓库复刻（fork）自上游仓库 **[boommilk1996-milk/codex-api-switch](https://github.com/boommilk1996-milk/codex-api-switch)**，并把实现语言从 Python 重写为 Go。
>
> 本工具是非官方工具，与 OpenAI 无关。上游原始说明已完整保留在 [docs/upstream-README.md](docs/upstream-README.md)，来源与改动记录见 [UPSTREAM.md](UPSTREAM.md)。

## 它解决什么问题

用第三方 Responses 兼容接口（例如 DeepSeek）推进过的会话，rollout 文件里会记录**明文推理内容**，以及第三方写入的 `encrypted_content` 占位符。切回官方接口继续这个会话时，官方会拒绝回放这段历史：

```text
Invalid 'input[N].content': array too long. Expected an array with maximum length 0, but got an array with length 1 instead.
The encrypted content ... could not be verified.
```

还有一种更隐蔽的情况：Codex 用 `thread_history_*.sqlite` 缓存已解析的历史，缓存里记录的是 rollout 文件的**字节偏移**。如果修复时改变了文件长度，缓存偏移就会失效，重新打开会话会报：

```text
invalid paginated history lineage for <会话id>: cutoff byte offset is past the source rollout
```

这两种现象都不是对话丢失，而是历史格式与回放接口不兼容。本工具会清掉不兼容的字段，同时保证文件长度不变，并让 Codex 重新解析受影响的会话。

## 会修什么、不会修什么

会修：

- 推理项里的明文 `content` 置为空数组
- 第三方写入的、无法被官方校验的 `encrypted_content` 移除（官方 `gAAAAA` 开头的密文原样保留）
- 一个会话对应的全部 rollout（初始文件加上每次续写）一起处理
- 刷新 `thread_history_*.sqlite` 里命中会话的缓存行，让 Codex 重新计算字节偏移

不会做：

- 不改写、不切换、不读取任何 API Key
- 不改 `config.toml`，不切换服务商，不做"切到 DeepSeek / 切回 OpenAI"
- 不改动对话正文、工具调用、摘要和元数据
- 不处理子任务、评审等非用户主任务之外的历史

## 安装

需要 Go 1.25 或更高版本。

```bash
# 方式一：一键安装到 ~/.local/bin
bash install.sh

# 方式二：用 make
make install PREFIX="$HOME/.local"

# 方式三：只编译，不安装
make build        # 生成当前目录下的 codex-session-doctor
```

## 使用

先体检，看看有多少会话需要修复：

```bash
codex-session-doctor doctor
```

修复前**完全退出 Codex（Cmd+Q）**，然后先预演一次：

```bash
codex-session-doctor repair 01a0c369-caf2-71c1-bdf1-a1f56cd46f00 --dry-run
```

确认无误后正式修复：

```bash
codex-session-doctor repair 01a0c369-caf2-71c1-bdf1-a1f56cd46f00 -y
```

或者一次修复全部受影响的会话：

```bash
codex-session-doctor repair --all --dry-run
codex-session-doctor repair --all -y
```

修完重新打开 Codex 并继续该会话即可。

### `-y` 是什么意思

`-y` 是 `--yes` 的缩写，只表示**跳过"是否继续"的确认提示**，方便脚本和批量操作。它不代表强制修复。真正绕过安全检查的是 `--force`：只有加了它，工具才会在检测到 Codex 正在运行时继续写入。

### 参数说明

| 参数 | 作用 |
| --- | --- |
| `<会话id>` | 修复指定会话；可以直接粘贴会话 UUID，工具会自动找到它的全部 rollout |
| `--all` | 修复所有受影响的会话 |
| `--dry-run` | 只列出将要改动的内容，不写任何文件 |
| `-y`, `--yes` | 跳过确认提示 |
| `--force` | 即使 Codex 正在运行也继续写入（不建议） |
| `--drop-foreign-reasoning` | 整行删除带非官方加密内容的推理项，会改变文件长度，必须配合刷新历史投影 |
| `--no-refresh-history` | 不清理历史投影缓存，仅用于排查问题 |

### 退出码

| 退出码 | 含义 |
| --- | --- |
| 0 | 成功 |
| 1 | 修复失败 |
| 2 | 参数用法错误 |
| 3 | 检测到 Codex 正在运行，已拒绝写入 |

## 安全设计

- **写入前一定备份。** 每个被修复的 rollout 都会原样复制到 `~/.codex/backups/codex-session-doctor/repair-<时间戳>/`，并附上 SHA-256 清单；如果存在历史投影库，它的数据库本体以及 `-wal`、`-shm` 边车文件也会一起进快照。
- **字节长度不变。** 正常修复通过补空格保持每一行的原始长度，因此 Codex 缓存的字节偏移继续有效。万一重写后变长，工具会直接报错并放弃写入，而不是写出一个偏移失效的文件。
- **运行中拒绝写入。** 桌面端正在运行时可能覆盖写这些文件，工具会检测并返回退出码 3，提示你先退出 Codex。
- **只碰该碰的字段。** 无关的行按字节原样保留；数字按原始字面量输出，避免大整数被浮点化。
- **备份失败就不动手。** 历史投影库备份失败时，工具会保留数据库原样并给出警告。

回滚方法见 [docs/rollback.md](docs/rollback.md)。

## 与上游的差异

上游 `codex-api-switch` 是一个服务商切换工具，本仓库只保留它的会话修复能力：

| 功能 | 上游 | 本仓库 |
| --- | --- | --- |
| 修复无法续聊的会话 | 有 | 有，并扩展了多 rollout、密文清理与历史投影刷新 |
| 切到第三方服务商 | 有 | 已移除 |
| 切回官方服务商 | 有 | 已移除 |
| 同步历史标签 / 会话模型 | 有 | 已移除 |
| 存储 API Key | 有 | 已移除 |
| 语言 | Python | Go |
| 配套界面 | macOS JXA 应用、PowerShell 菜单 | 已移除，只保留命令行 |

## 开发

```bash
make check    # go vet + go test
make test     # 只跑测试
make fmt      # 格式化
```

测试覆盖了字节长度不变、官方与伪加密文的区分、无关行不被改写、大整数字面量保持、重复修复幂等、多 rollout 会话的父子 id 解析、只清理命中会话的历史投影、运行中拒绝写入与 `--force` 行为，以及端到端的修复流程。

实现思路和关键约束见 [docs/architecture.md](docs/architecture.md)。

## 已知边界

- 第三方写入的加密内容无法被还原成官方可验证的密文，工具只能移除它，让其余历史可以回放；隐藏的推理过程本身不会恢复。
- 修复依赖本地文件结构（`state_5.sqlite`、`thread_history_*.sqlite`、`sessions/` 目录）。Codex 若在未来版本调整这些结构，需要同步更新本工具。
- `state_5.sqlite` 短时间无法读取时，工具会退化为只扫描 `sessions/` 与 `archived_sessions/` 目录并给出警告。
