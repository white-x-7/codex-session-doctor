# Rollback

Every repair writes a timestamped backup directory under:

```text
~/.codex/backups/codex-api-switch/repair-<timestamp>/
```

The backup contains the original rollout paths and `manifest.json` with SHA-256
checksums. When a history projection database is present, its SQLite file and
available `-wal` and `-shm` sidecars are copied into the same snapshot.

If a repair must be reverted, fully quit Codex, identify the matching files in
the manifest, copy those backup files back to their original paths, restore the
matching history database and sidecars together, and then reopen Codex. Keep
the backup until the restored task has been opened and continued successfully.

The `invalid paginated history lineage` error means that a cached byte offset
is past the current rollout length. This repository prevents that during the
normal repair by preserving byte lengths and drops the affected projection rows
when a size-changing operation is explicitly requested.

Encrypted content produced by a foreign provider cannot be reconstructed by
this tool. The repair removes an unverifiable placeholder so the official API
can replay the rest of the history; it cannot recover the original hidden
reasoning token.
