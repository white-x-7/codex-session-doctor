#!/usr/bin/env python3
"""Focused regression tests for the cross-provider session repair path."""

from __future__ import annotations

import importlib.machinery
import importlib.util
import json
import sqlite3
import tempfile
from pathlib import Path


ROOT = Path(__file__).parent


def load_switch_module():
    """Load the extension-less CLI script as a module."""
    script = ROOT / "codex-api-switch"
    loader = importlib.machinery.SourceFileLoader("codex_api_switch", str(script))
    spec = importlib.util.spec_from_loader("codex_api_switch", loader)
    module = importlib.util.module_from_spec(spec)
    loader.exec_module(module)
    return module


switch = load_switch_module()


def check(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def test_repair_preserves_size_and_encrypted_blobs() -> None:
    official = "gAAAAABofficial-example"
    foreign = "8ec8e468-e6a3-4513-a1f0-9fbb4f189148-0"
    with tempfile.TemporaryDirectory(prefix="codex-repair-test-") as tmp:
        path = Path(tmp) / "rollout.jsonl"
        rows = [
            {
                "type": "response_item",
                "payload": {
                    "type": "reasoning",
                    "content": [{"type": "reasoning_text", "text": "secret"}],
                    "encrypted_content": foreign,
                },
            },
            {
                "type": "response_item",
                "payload": {
                    "type": "reasoning",
                    "content": [],
                    "encrypted_content": official,
                },
            },
        ]
        path.write_bytes(b"\n".join(json.dumps(row).encode() for row in rows) + b"\n")
        before = path.stat().st_size
        result = switch.rewrite_reasoning_items(path)
        check(result["content_emptied"] == 1, "plaintext reasoning was emptied")
        check(result["encrypted_stripped"] == 1, "foreign encrypted content was stripped")
        check(path.stat().st_size == before, "normal repair preserved byte length")
        parsed = [json.loads(line) for line in path.read_text().splitlines()]
        check(parsed[0]["payload"]["content"] == [], "content became an empty array")
        check("encrypted_content" not in parsed[0]["payload"], "foreign blob was removed")
        check(parsed[1]["payload"]["encrypted_content"] == official, "official blob was kept")


def test_rollout_ids_include_parent_and_segment() -> None:
    with tempfile.TemporaryDirectory(prefix="codex-rollout-id-test-") as tmp:
        parent = "01parent"
        segment = "01segment"
        path = Path(tmp) / f"rollout-2026-09-23T00-00-00-{parent}_{segment}.jsonl"
        path.write_text(json.dumps({"payload": {"session_id": parent}}) + "\n")
        check(switch.rollout_thread_ids(path) == {parent, segment}, "both rollout IDs resolved")


def test_projection_refresh_removes_only_matching_ids() -> None:
    with tempfile.TemporaryDirectory(prefix="codex-history-test-") as tmp:
        db = Path(tmp) / "thread_history_1.sqlite"
        conn = sqlite3.connect(db)
        for table in switch.HISTORY_TABLES:
            conn.execute(f"CREATE TABLE {table} (thread_id TEXT, value TEXT)")
            conn.executemany(
                f"INSERT INTO {table} VALUES (?, ?)",
                [("parent", "old"), ("segment", "old"), ("keep", "new")],
            )
        conn.commit()
        conn.close()
        original = switch.history_db_path
        switch.history_db_path = lambda: db
        try:
            deleted, errors = switch.refresh_history_projection({"parent", "segment"}, None)
        finally:
            switch.history_db_path = original
        check(not errors, "projection refresh completed")
        check(deleted == len(switch.HISTORY_TABLES) * 2, "all parent and segment rows were removed")
        conn = sqlite3.connect(db)
        remaining = conn.execute("SELECT thread_id FROM thread_items").fetchall()
        conn.close()
        check(remaining == [("keep",)], "unrelated projection rows were retained")


if __name__ == "__main__":
    test_repair_preserves_size_and_encrypted_blobs()
    test_rollout_ids_include_parent_and_segment()
    test_projection_refresh_removes_only_matching_ids()
    print("REPAIR TESTS PASSED")
