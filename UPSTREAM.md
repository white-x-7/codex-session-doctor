# Upstream and local changes

This private repository is based on the upstream project:

- Repository: https://github.com/boommilk1996-milk/codex-api-switch
- Imported commit: `dd45014b6ee1272167aaed302131325f4048c0f8`
- Imported on: 2026-09-23

The upstream project is a third-party tool and is not an OpenAI product. The
unmodified upstream README is retained as `README.upstream.md`.

Local changes in this repository focus on restoring Codex sessions after they
were advanced through a third-party Responses-compatible provider:

1. JSONL rewrites preserve each line's byte length by padding shortened lines.
2. Foreign `encrypted_content` placeholders are removed while official blobs
   beginning with `gAAAAA` are preserved.
3. A session ID resolves to every initial and continued rollout segment.
4. Cached rows in `thread_history_*.sqlite` are refreshed for the parent and
   segment IDs after repair.
5. Active and archived rollout trees are scanned, and history database sidecars
   are included in backups.

See [docs/repair.md](docs/repair.md) for the safe command workflow and
[docs/rollback.md](docs/rollback.md) for recovery.
