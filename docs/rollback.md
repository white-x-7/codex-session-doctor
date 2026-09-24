# 回滚指南

每一次修复都会写入一个独立的快照目录，回滚就是把快照里的文件放回原位。

## 备份放在哪

```text
~/.codex/backups/codex-session-doctor/repair-<时间戳>/
```

目录里包含：

| 内容 | 说明 |
| --- | --- |
| `sessions/...`、`archived_sessions/...` | 被修复的 rollout 文件原件，目录结构与主目录一致 |
| `manifest.json` | 每个 rollout 的原始路径、SHA-256 与快照路径 |
| `thread_history_<版本>.sqlite` | 历史投影库在清理前的副本 |
| `thread_history_<版本>.sqlite-wal`、`-shm` | SQLite 边车文件，存在时才复制 |

## 怎么回滚

1. **完全退出 Codex（Cmd+Q）。** 桌面端运行时会覆盖写这些文件。
2. 打开 `manifest.json`，找到你要回滚的会话对应的条目。
3. 把快照里的 rollout 文件复制回 `manifest.json` 里记录的原始路径。
4. 如果快照里有历史投影库，把数据库本体和它的 `-wal`、`-shm` 一起复制回 `~/.codex/`。**一定要一起放回**，只放主库会丢掉边车文件里尚未合并的事务。
5. 重新打开 Codex，确认该会话可以正常打开，再决定是否删除备份。

在确认恢复成功之前，不要删除快照目录。

## 想撤销投影清理、但保留修复

如果只是历史投影被清了、想回到清理前的状态，从快照里取回历史数据库及其边车文件即可，rollout 文件保持修复后的版本。Codex 会在下次打开会话时重新解析并重建缓存。

## 常见报错与快照的关系

`invalid paginated history lineage ... cutoff byte offset is past the source rollout` 表示缓存里的字节偏移超出了当前文件长度。它通常来自"文件被改短但缓存没刷新"。本工具的正常修复路径会保持字节长度，因此不会产生这种偏移失效。

## 第三方加密内容无法还原

第三方写入的 `encrypted_content` 是无法被官方接口验证的占位符，工具只能移除它，让其余历史可以回放。隐藏的推理内容本身不会因为回滚而恢复，因为它在写入时就不是官方格式。
