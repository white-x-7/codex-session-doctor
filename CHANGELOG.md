# Changelog

## Private repair fork — 2026-09-23

- Imported the upstream `boommilk1996-milk/codex-api-switch` project.
- Added byte-preserving JSONL rewrites to avoid stale paginated-history offsets.
- Removed unverifiable foreign `encrypted_content` placeholders while retaining
  official `gAAAAA...` blobs.
- Resolved parent and continued rollout segment IDs together.
- Refreshed `thread_history_*.sqlite` projection rows for every affected ID.
- Added archived-session scanning and SQLite sidecar backups.
- Added focused repair regression tests and Chinese operational/rollback docs.
