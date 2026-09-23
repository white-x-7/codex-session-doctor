# 实现说明

本文记录代码结构与几条不能违反的约束。改动本工具前请先读这一页。

## 数据流向

```text
~/.codex/
├── state_5.sqlite            # threads 表记录每个会话的 rollout_path（只读）
├── thread_history_*.sqlite   # 已解析历史的缓存，含字节偏移（需要清理命中行）
├── sessions/                 # 当前会话的 rollout JSONL
├── archived_sessions/        # 已归档会话的 rollout JSONL
└── backups/codex-session-doctor/   # 本工具写入的快照
```

一次 `repair` 的流程：

1. 收集 rollout 路径（目录扫描 + `state_5.sqlite` 记录）
2. 按会话 id 或 `--all` 过滤出候选文件
3. 扫描每个文件，统计不兼容的推理项；没有问题的文件直接跳过
4. 把将要改动的文件复制进快照，写入 SHA-256 清单
5. 逐文件重写：清空明文 `content`、移除非官方 `encrypted_content`、补齐空格保持字节长度
6. 汇总命中会话的 id（父 id 与每个续写分段 id）
7. 删除 `thread_history_*.sqlite` 中这些 id 的缓存行，让 Codex 重新解析

## 包结构

| 包 | 职责 |
| --- | --- |
| `internal/codex` | 主目录解析、桌面端进程检测、`state_5.sqlite` 只读查询 |
| `internal/rollout` | rollout 解析、推理项统计、字节保长重写、会话 id 解析 |
| `internal/history` | 历史投影库定位、备份与缓存行清理 |
| `internal/backup` | 快照目录、文件复制、SHA-256 清单 |
| `internal/repair` | 扫描与修复的编排，输出结果汇总 |
| `internal/cli` | 命令行解析、中文输出、退出码 |

## 不能违反的约束

### 一、正常修复必须保持字节长度

Codex 用 `thread_history_*.sqlite` 缓存已解析的历史，缓存里记录的是 rollout 文件里的**字节偏移**。如果重写让文件变短而缓存没更新，下次打开会话就会得到：

```text
invalid paginated history lineage: cutoff byte offset is past the source rollout
```

因此重写单行后，如果结果比原来短，就在行尾补空格；JSONL 的行尾空白不影响解析。如果结果反而更长，工具直接报错并放弃写入，而不是写出一个偏移失效的文件。写入前还会再比对一次整体长度，长度不符就拒绝落盘。

### 二、只移除官方接口无法接受的东西

官方写入的 `encrypted_content` 是 `gAAAAA` 开头的 Fernet 密文，必须原样保留。第三方写入的是别的形态（例如 `8ec8e468-...-0` 这样的占位符），官方无法校验，必须移除。判定逻辑集中在 `rollout.IsForeignEncryptedContent`，不要在别处重复实现。

### 三、一个会话可能对应多个 rollout

续写过的会话会写成 `rollout-<时间>-<会话id>_<分段id>.jsonl`。Codex 为会话 id 和每个分段 id 分别维护分页游标，所以修复时必须：

- 把同一会话的全部 rollout 一起处理
- 对父 id 与每个分段 id 都清理缓存行

只清理父 id 会留下未更新的分段游标，仍然报偏移错误。

### 四、运行中不写入

桌面端运行时会覆盖写这些文件。检测到进程存在时返回退出码 3 并提示用户先退出，只有显式 `--force` 才继续。

### 五、备份先于改动

任何写入之前都要先落快照。数据库备份失败时保持数据库原样，不做"先改再补备份"这种事。

### 六、不要破坏数字

解析 JSON 时使用 `json.Number`。若用 `map[string]any` 直接解析，整数会变成 `float64`，大整数（例如毫秒时间戳）会丢精度并被改写成科学计数法，等于篡改了用户的历史数据。

## 相关环境变量

| 变量 | 用途 |
| --- | --- |
| `CODEX_SESSION_DOCTOR_HOME` | 指向模拟主目录，测试与隔离运行使用；设置后不再检测真实桌面进程 |
| `CODEX_HOME` | 与 Codex 自身一致的覆盖变量 |

## 测试策略

- `internal/rollout`：字节长度保持、无关行按字节不变、官方与伪密文的区分、幂等性、`--drop-foreign-reasoning` 的整行删除、大整数字面量保持
- `internal/history`：版本号选择、只清理命中 id、边车文件进快照、备份失败时不动数据库
- `internal/cli`：`doctor` 输出、预演不写文件、修复保持长度并清理投影、多 rollout 会话的父子 id、运行中拒绝写入、`--force` 放行、未知会话与参数用法错误、重复修复的无操作路径

进程检测在 `cli.Env.IsRunning` 处可注入，测试因此不依赖本机是否真的开着 Codex。
