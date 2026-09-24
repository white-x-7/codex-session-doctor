# 会话修复指南

本文说明什么时候需要修复、修复会改动什么，以及推荐的执行步骤。

## 什么时候需要修复

出现下列任一情况时，说明会话历史里有官方接口不接受的字段：

```text
Invalid 'input[N].content': array too long. Expected an array with maximum length 0, but got an array with length 1 instead.
The encrypted content ... could not be verified.
invalid paginated history lineage for <会话id>: cutoff byte offset is past the source rollout
```

通常的触发路径是：用第三方 Responses 兼容接口继续过一个会话，之后切回官方接口再打开它。

## 修复前

1. 先在 Codex 里找到出问题的会话，复制它的会话 id。会话 id 是一段 UUID，形如 `01a0c369-caf2-71c1-bdf1-a1f56cd46f00`。
2. **完全退出 Codex（Cmd+Q）**。工具会在检测到 Codex 运行时拒绝写入，并返回退出码 3。
3. 跑一次体检确认范围：

   ```bash
   codex-session-doctor doctor
   ```

## 修复一个会话

先预演，确认将要改动的内容：

```bash
codex-session-doctor repair 01a0c369-caf2-71c1-bdf1-a1f56cd46f00 --dry-run
```

预演只读，输出类似：

```text
会话 '01a0c369-caf2-71c1-bdf1-a1f56cd46f00' 匹配到 rollout：3 个
  rollout-2026-09-22T09-21-19-01a0c369-...jsonl
  rollout-2026-09-22T14-55-59-01a0c369-...jsonl
  rollout-2026-09-22T15-18-39-01a0c369-...jsonl
需要修复的 rollout：1 个
  rollout-2026-09-22T15-18-39-01a0c369-...jsonl：1 处推理明文待清空，1 处非官方加密内容待移除（共 12 个推理项）
预演完成：没有修改任何文件。
```

确认后正式修复：

```bash
codex-session-doctor repair 01a0c369-caf2-71c1-bdf1-a1f56cd46f00 -y
```

输出类似：

```text
已修复 1 个 rollout：
  清空推理明文         : 1
  移除非官方加密内容   : 1
  字节长度保持不变     : 214 字节填充
  历史投影缓存         : 清理 4 行，Codex 将重新读取
备份：/Users/你/.codex/backups/codex-session-doctor/repair-20260923-173012
```

然后重新打开 Codex，找到该会话继续对话。

## 批量修复

```bash
codex-session-doctor repair --all --dry-run
codex-session-doctor repair --all -y
```

`--all` 会扫描 `~/.codex/sessions/`、`~/.codex/archived_sessions/` 以及 `state_5.sqlite` 里记录的路径。

## `-y` 与 `--force` 的区别

`-y`（即 `--yes`）只是跳过确认提示，让命令可以无人值守地跑完。它不改变任何安全判断。

`--force` 才会绕过"Codex 正在运行就不写入"的保护。运行中的桌面端可能覆盖写这些文件，所以除非你明确知道自己在做什么，否则不要加 `--force`。

## 修复会改动什么

| 对象 | 改动 |
| --- | --- |
| 推理项的 `content` | 明文内容置为空数组 `[]` |
| 非官方 `encrypted_content` | 整字段移除 |
| 官方 `encrypted_content`（`gAAAAA` 开头） | 原样保留 |
| 用户消息、助手消息、工具调用、摘要 | 原样保留 |
| rollout 文件长度 | 保持不变（不足处补空格） |
| `thread_history_*.sqlite` | 删除命中会话的缓存行，让 Codex 重新解析 |
| `state_5.sqlite` | 只读，不修改 |

## 高级选项

`--drop-foreign-reasoning` 会把带非官方加密内容的推理项整行删除。它会改变文件长度，因此工具必须刷新历史投影，不能和 `--no-refresh-history` 同时使用。只有在保持长度的方式仍然无法解决问题时才需要它。

`--no-refresh-history` 跳过历史投影清理，只用于排查问题。正常修复不要加，否则可能重新出现字节偏移相关的报错。

## 修完之后

如果仍然报错，可以：

1. 用 `codex-session-doctor doctor` 确认该会话已经不在"需要修复"的列表里；
2. 用 [rollback.md](rollback.md) 里的方法从备份回滚，回到修复前的状态；
3. 带上 `doctor` 的输出和完整报错信息再排查。
