# Session repair

Use `repair` after a Codex session has been continued through a third-party
provider and then needs to continue through the official provider again.

The third-party rollout can contain plaintext reasoning content or a provider
placeholder in `encrypted_content`. The official API rejects those fields when
it replays the old history. The repair keeps the conversation messages and
removes only the incompatible reasoning fields.

## Safe workflow

1. Fully quit Codex with `Cmd+Q`. The command refuses to write while Codex is
   running.
2. Preview the matching rollout segments:

   ```bash
   codex-api-switch repair <session-id> --dry-run
   ```

3. Apply the repair and skip the confirmation prompt:

   ```bash
   codex-api-switch repair <session-id> -y
   ```

   Replace `<session-id>` with the UUID only; do not type the angle brackets.
   `-y` means `--yes` and only skips the confirmation prompt.

4. Reopen Codex and resume the task.

The command automatically finds every rollout whose filename belongs to that
session, including continued segments and archived rollouts. It creates a
backup under `~/.codex/backups/codex-api-switch/` before changing files and
refreshes the cached history projection for the parent and segment IDs.

## What is preserved

- Official encrypted blobs whose value starts with `gAAAAA`.
- User and assistant messages, tool calls, summaries, and metadata.
- Each rollout file's byte length during the normal repair path. This prevents
  stale byte offsets in Codex's paginated-history cache.

## Advanced options

`--all` repairs every affected rollout on the machine. Preview it first:

```bash
codex-api-switch repair --all --dry-run
codex-api-switch repair --all -y
```

`--drop-foreign-reasoning` removes entire reasoning lines. It changes file
length and therefore requires the history projection refresh; use it only when
the normal byte-preserving repair is insufficient. `--force` bypasses safety
checks and should not be part of the normal workflow.
