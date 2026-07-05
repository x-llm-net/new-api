#!/usr/bin/env python3
"""
Import llm-hub radar probe runs into the local X-LLM group stability samples.

This script is intentionally local-only:
- reads the llm-hub radar SQLite/libSQL data through SSH
- writes only to a SQLite DB under .local-test
- refuses to write outside the repository
"""

from __future__ import annotations

import argparse
import json
import shutil
import sqlite3
import subprocess
import sys
import time
from pathlib import Path


LLMHUB_DB_PATH = "/var/lib/docker/volumes/llmhub-radar-libsql-data/_data/iku.db/dbs/default/data"

GROUP_TARGETS = [
    {
        "group": "Claude-Kiro\u9ad8\u7f13",
        "target_id": 24,
        "source": "deepkey / Anthropic / claude-kiro",
    },
    {
        "group": "Claude-\u6ee1\u8840",
        "target_id": 18,
        "source": "X-LLM / Anthropic / Claude-\u6ee1\u8840",
    },
    {
        "group": "Claude-\u4f01\u4e1a\u7ea7\u7a33\u5b9a",
        "target_id": 23,
        "source": "X-LLM / Anthropic / CC\u4f18\u8d28\u4f01\u4e1a\u7ea7",
    },
    {
        "group": "Claude-Max",
        "target_id": 19,
        "source": "X-LLM / Anthropic / Claude-Max",
    },
    {
        "group": "Gemini-\u4e13\u7528\u5206\u7ec4",
        "target_id": 22,
        "source": "X-LLM / Gemini / Gemini-\u4e13\u7528\u5206\u7ec4",
    },
    {
        "group": "Codex-\u7a33\u5b9a\u5b98\u6c60",
        "target_id": 12,
        "source": "X-LLM / OpenAI / Codex-\u7a33\u5b9a\u5b98\u6c60",
    },
    {
        "group": "Codex-Plus",
        "target_id": 29,
        "source": "deepkey / OpenAI / gpt",
    },
    {
        "group": "Codex-Pro",
        "target_id": 21,
        "source": "X-LLM / OpenAI / CodeX-Pro",
    },
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Import llm-hub radar runs into local group stability samples.",
    )
    parser.add_argument("--ssh-host", default="llm-hub", help="SSH host alias for llm-hub.")
    parser.add_argument("--db", default=".local-test/one-api.db", help="Local SQLite database path.")
    parser.add_argument("--days", type=int, default=7, help="Days of radar history to import.")
    parser.add_argument("--replace", action="store_true", help="Delete existing local stability samples before import.")
    parser.add_argument("--dry-run", action="store_true", help="Fetch and summarize without writing.")
    return parser.parse_args()


def repo_root() -> Path:
    return Path(__file__).resolve().parents[1]


def ensure_local_db(path: Path) -> Path:
    root = repo_root().resolve()
    resolved = path.resolve()
    if root not in [resolved, *resolved.parents]:
        raise SystemExit(f"Refusing to write outside repo: {resolved}")
    if ".local-test" not in resolved.parts:
        raise SystemExit(f"Refusing to write a DB outside .local-test: {resolved}")
    return resolved


def backup_local_db(db_path: Path) -> Path | None:
    if not db_path.exists():
        return None
    backup_dir = db_path.parent / "backups"
    backup_dir.mkdir(parents=True, exist_ok=True)
    stamp = time.strftime("%Y%m%d-%H%M%S")
    backup_path = backup_dir / f"{db_path.name}.before-llmhub-radar-{stamp}.bak"
    shutil.copy2(db_path, backup_path)
    return backup_path


def fetch_remote_runs(ssh_host: str, days: int) -> list[dict[str, object]]:
    mapping_json = json.dumps(GROUP_TARGETS, ensure_ascii=True)
    remote_script = f"""
import json
import sqlite3
import time

db_path = {LLMHUB_DB_PATH!r}
days = {int(days)}
mapping = json.loads({mapping_json!r})
target_ids = [item["target_id"] for item in mapping]
cutoff = int(time.time()) - days * 24 * 3600

con = sqlite3.connect("file:" + db_path + "?mode=ro", uri=True)
con.row_factory = sqlite3.Row
cur = con.cursor()
placeholders = ",".join("?" for _ in target_ids)
query = f'''
SELECT
  r.id AS run_id,
  r.target_id,
  r.started_at,
  r.finished_at,
  r.success,
  r.http_status,
  r.error_type,
  r.first_token_ms,
  r.total_latency_ms,
  t.name AS target_name,
  t.display_name AS target_display_name,
  t.model_name,
  p.display_name AS provider_name,
  c.billing_group,
  c.model_group
FROM radar_probe_run r
JOIN radar_probe_target t ON t.id = r.target_id
LEFT JOIN radar_provider p ON p.id = t.provider_id
LEFT JOIN radar_credential c ON c.id = t.credential_id
WHERE r.target_id IN ({{placeholders}})
  AND r.started_at >= ?
ORDER BY r.started_at ASC, r.id ASC
'''
rows = [dict(row) for row in cur.execute(query, [*target_ids, cutoff])]
con.close()
print(json.dumps(rows, ensure_ascii=False))
"""
    result = subprocess.run(
        ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", ssh_host, "python3", "-"],
        input=remote_script,
        text=True,
        capture_output=True,
        encoding="utf-8",
        timeout=180,
    )
    if result.returncode != 0:
        sys.stderr.write(result.stderr)
        raise SystemExit(result.returncode)
    return json.loads(result.stdout)


def normalize_started_at(value: object) -> int:
    if value is None:
        return 0
    ts = int(value)
    if ts > 9_999_999_999:
        return ts // 1000
    return ts


def summarize(rows: list[dict[str, object]]) -> None:
    by_target: dict[int, dict[str, int]] = {}
    for row in rows:
        target_id = int(row["target_id"])
        bucket = by_target.setdefault(target_id, {"runs": 0, "ok": 0})
        bucket["runs"] += 1
        if int(row["success"] or 0) == 1:
            bucket["ok"] += 1

    for item in GROUP_TARGETS:
        stats = by_target.get(int(item["target_id"]), {"runs": 0, "ok": 0})
        runs = stats["runs"]
        ok = stats["ok"]
        rate = (ok / runs * 100) if runs else 0
        print(
            f"{item['group']} <- target {item['target_id']} ({item['source']}): "
            f"{ok}/{runs} = {rate:.2f}%"
        )


def import_rows(db_path: Path, rows: list[dict[str, object]], replace: bool) -> None:
    group_by_target = {int(item["target_id"]): item["group"] for item in GROUP_TARGETS}
    now = int(time.time())
    samples = []
    for row in rows:
        target_id = int(row["target_id"])
        group = group_by_target.get(target_id)
        if not group:
            continue
        tested_at = normalize_started_at(row["started_at"])
        if tested_at <= 0:
            continue
        success = 1 if int(row["success"] or 0) == 1 else 0
        error_code = "" if success else str(row["error_type"] or "radar_failed")
        response_time_ms = int(row["total_latency_ms"] or 0)
        samples.append(
            (
                f"llmhub:{target_id}:{row['run_id']}",
                target_id,
                group,
                success,
                error_code,
                response_time_ms,
                1,
                1 if success else 2,
                tested_at,
                now,
            )
        )

    conn = sqlite3.connect(db_path)
    try:
        conn.execute("BEGIN")
        if replace:
            conn.execute("DELETE FROM xllm_group_stability_samples")
        conn.executemany(
            """
            INSERT INTO xllm_group_stability_samples (
              task_id,
              channel_id,
              group_names,
              success,
              error_code,
              response_time_ms,
              channel_status_before,
              channel_status_after,
              tested_at,
              created_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            samples,
        )
        conn.commit()
    except Exception:
        conn.rollback()
        raise
    finally:
        conn.close()

    print(f"imported {len(samples)} samples into {db_path}")


def main() -> None:
    args = parse_args()
    if args.days <= 0:
        raise SystemExit("--days must be positive")

    db_path = ensure_local_db(repo_root() / args.db)
    rows = fetch_remote_runs(args.ssh_host, args.days)
    print(f"fetched {len(rows)} llm-hub radar runs from the last {args.days} days")
    summarize(rows)

    if args.dry_run:
        print("dry-run: no local DB writes")
        return

    if not db_path.exists():
        raise SystemExit(f"Local DB does not exist: {db_path}")

    backup = backup_local_db(db_path)
    if backup:
        print(f"backup: {backup}")
    import_rows(db_path, rows, args.replace)


if __name__ == "__main__":
    main()
