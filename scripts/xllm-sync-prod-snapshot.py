#!/usr/bin/env python3
"""
Sync a read-only production data snapshot into the local SQLite database.

This script never opens a production database connection from the local machine.
It reads production data through SSH by running SELECT statements inside the
production MySQL container, then replaces the local SQLite data needed for
homepage stability testing.
"""

from __future__ import annotations

import argparse
import base64
import csv
import json
import os
import shutil
import sqlite3
import subprocess
import sys
import time
from pathlib import Path


CORE_TABLES = [
    "abilities",
    "channels",
    "models",
    "options",
    "setups",
    "users",
]

OPTIONAL_TABLES = [
]

LOG_TABLE = "logs"
MODEL_TEST_TOKEN_NAME = "模型测试"
DEFAULT_GROUP_ORDER = [
    "Claude-Kiro\u9ad8\u7f13",
    "Claude-\u6ee1\u8840",
    "Claude-\u4f01\u4e1a\u7ea7\u7a33\u5b9a",
    "Claude-Max",
    "Gemini-\u4e13\u7528\u5206\u7ec4",
    "Codex-\u7a33\u5b9a\u5b98\u6c60",
    "Codex-Plus",
    "Codex-Pro",
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Import a production snapshot into the local SQLite DB for X-LLM homepage testing.",
    )
    parser.add_argument("--ssh-host", default="x-llm-net", help="SSH host alias for production.")
    parser.add_argument("--compose-dir", default="/opt/llm-hub", help="Production compose directory.")
    parser.add_argument("--db", default=".local-test/one-api.db", help="Local SQLite database path.")
    parser.add_argument("--days", type=int, default=10, help="Number of days of logs to import.")
    parser.add_argument(
        "--keep-local-root-password",
        action=argparse.BooleanOptionalAction,
        default=True,
        help="After importing users, keep the previous local root password hash.",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Fetch counts only and do not write the local database.",
    )
    return parser.parse_args()


def repo_root() -> Path:
    return Path(__file__).resolve().parents[1]


def ensure_local_target(path: Path) -> Path:
    root = repo_root().resolve()
    resolved = path.resolve()
    if root not in [resolved, *resolved.parents]:
        raise SystemExit(f"Refusing to write outside repo: {resolved}")
    if ".local-test" not in resolved.parts:
        raise SystemExit(f"Refusing to replace a DB outside .local-test: {resolved}")
    return resolved


def run_remote_mysql(ssh_host: str, compose_dir: str, sql: str) -> str:
    command = (
        f"cd {shell_quote(compose_dir)} && "
        "docker compose exec -T mysql sh -lc "
        "'mysql --default-character-set=utf8mb4 "
        "-u\"$MYSQL_USER\" -p\"$MYSQL_PASSWORD\" \"$MYSQL_DATABASE\" "
        "--batch --skip-column-names'"
    )
    result = subprocess.run(
        ["ssh", ssh_host, command],
        input=sql,
        text=True,
        capture_output=True,
        encoding="utf-8",
        timeout=180,
    )
    if result.returncode != 0:
        sys.stderr.write(result.stderr)
        raise SystemExit(result.returncode)
    return result.stdout


def shell_quote(value: str) -> str:
    return "'" + value.replace("'", "'\"'\"'") + "'"


def mysql_ident(name: str) -> str:
    return "`" + name.replace("`", "``") + "`"


def decode_mysql_cell(value: str | None) -> str | None:
    if value is None or value == r"\N" or value == "NULL":
        return None
    result: list[str] = []
    index = 0
    while index < len(value):
        char = value[index]
        if char != "\\" or index + 1 >= len(value):
            result.append(char)
            index += 1
            continue
        index += 1
        escaped = value[index]
        result.append(
            {
                "0": "\0",
                "b": "\b",
                "n": "\n",
                "r": "\r",
                "t": "\t",
                "Z": "\x1a",
                "\\": "\\",
            }.get(escaped, escaped),
        )
        index += 1
    return "".join(result)


def fetch_table_rows(ssh_host: str, compose_dir: str, table: str, where: str = "") -> tuple[list[str], list[dict[str, str | None]]]:
    columns_sql = (
        "SELECT COLUMN_NAME FROM information_schema.COLUMNS "
        f"WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = '{table}' "
        "ORDER BY ORDINAL_POSITION;"
    )
    columns = [line for line in run_remote_mysql(ssh_host, compose_dir, columns_sql).splitlines() if line]
    if not columns:
        return [], []

    select_list = ", ".join(mysql_ident(column) for column in columns)
    sql = f"SELECT {select_list} FROM {mysql_ident(table)}"
    if where:
        sql += f" WHERE {where}"
    sql += ";"
    output = run_remote_mysql(ssh_host, compose_dir, sql)
    rows: list[dict[str, str | None]] = []
    reader = csv.reader(output.splitlines(), delimiter="\t", quoting=csv.QUOTE_NONE)
    for raw in reader:
        if len(raw) == 1 and raw[0] == "":
            continue
        if len(raw) != len(columns):
            raise RuntimeError(f"Unexpected column count for {table}: got {len(raw)}, expected {len(columns)}")
        rows.append({column: decode_mysql_cell(raw[index]) for index, column in enumerate(columns)})
    return columns, rows


def fetch_remote_counts(ssh_host: str, compose_dir: str, days: int) -> dict[str, int]:
    sql_parts = []
    for table in [*CORE_TABLES, *OPTIONAL_TABLES]:
        sql_parts.append(f"SELECT '{table}', COUNT(*) FROM {mysql_ident(table)};")
    sql_parts.append(f"SELECT 'logs_{days}d', COUNT(*) FROM logs WHERE created_at >= UNIX_TIMESTAMP(NOW() - INTERVAL {days} DAY);")
    sql_parts.append(
        "SELECT 'logs_model_test_same_nonempty', COUNT(*) FROM logs "
        f"WHERE created_at >= UNIX_TIMESTAMP(NOW() - INTERVAL {days} DAY) "
        "AND type=2 AND token_name=content AND token_name<>'';"
    )
    output = run_remote_mysql(ssh_host, compose_dir, "\n".join(sql_parts))
    counts: dict[str, int] = {}
    for line in output.splitlines():
        if not line:
            continue
        name, value = line.split("\t", 1)
        counts[name] = int(value)
    return counts


def local_table_columns(conn: sqlite3.Connection, table: str) -> list[str]:
    rows = conn.execute(f"PRAGMA table_info({quote_sqlite_ident(table)})").fetchall()
    return [row[1] for row in rows]


def quote_sqlite_ident(name: str) -> str:
    return '"' + name.replace('"', '""') + '"'


def table_exists(conn: sqlite3.Connection, table: str) -> bool:
    row = conn.execute(
        "SELECT 1 FROM sqlite_master WHERE type='table' AND name=?",
        (table,),
    ).fetchone()
    return row is not None


def backup_local_db(db_path: Path) -> Path | None:
    if not db_path.exists():
        return None
    backup_dir = db_path.parent / "backups"
    backup_dir.mkdir(parents=True, exist_ok=True)
    stamp = time.strftime("%Y%m%d-%H%M%S")
    backup_path = backup_dir / f"{db_path.name}.{stamp}.bak"
    shutil.copy2(db_path, backup_path)
    return backup_path


def capture_local_root_password(conn: sqlite3.Connection) -> tuple[int, str] | None:
    if not table_exists(conn, "users"):
        return None
    row = conn.execute(
        "SELECT id, password FROM users WHERE username='root' AND role=100 ORDER BY id LIMIT 1",
    ).fetchone()
    if row and row[1]:
        return int(row[0]), str(row[1])
    return None


def restore_local_root_password(conn: sqlite3.Connection, root_password: tuple[int, str] | None) -> None:
    if root_password is None or not table_exists(conn, "users"):
        return
    root_id, password_hash = root_password
    conn.execute("UPDATE users SET password=? WHERE id=?", (password_hash, root_id))


def clear_table(conn: sqlite3.Connection, table: str) -> None:
    if table_exists(conn, table):
        conn.execute(f"DELETE FROM {quote_sqlite_ident(table)}")


def insert_rows(conn: sqlite3.Connection, table: str, remote_columns: list[str], rows: list[dict[str, str | None]]) -> int:
    if not rows or not table_exists(conn, table):
        return 0
    local_columns = local_table_columns(conn, table)
    insert_columns = [column for column in remote_columns if column in local_columns]
    if not insert_columns:
        return 0
    placeholders = ",".join("?" for _ in insert_columns)
    column_sql = ", ".join(quote_sqlite_ident(column) for column in insert_columns)
    sql = f"INSERT INTO {quote_sqlite_ident(table)} ({column_sql}) VALUES ({placeholders})"
    normalized_rows = [normalize_row_for_sqlite(table, row) for row in rows]
    values = [tuple(row[column] for column in insert_columns) for row in normalized_rows]
    conn.executemany(sql, values)
    return len(values)


def normalize_row_for_sqlite(table: str, row: dict[str, str | None]) -> dict[str, str | None]:
    normalized = dict(row)
    if table == "channels":
        if not normalized.get("channel_info"):
            normalized["channel_info"] = "{}"
        return normalized
    if table != "users":
        return row
    for column in ["access_token", "aff_code"]:
        if normalized.get(column) == "":
            normalized[column] = None
    return normalized


def normalize_groups(group_value: str | None) -> list[str]:
    if not group_value:
        return []
    result: list[str] = []
    seen: set[str] = set()
    for part in group_value.split(","):
        group = part.strip()
        if group and group not in seen:
            seen.add(group)
            result.append(group)
    return result


def build_group_stability_samples(conn: sqlite3.Connection, model_test_token_name: str) -> int:
    if not table_exists(conn, "xllm_group_stability_samples"):
        return 0
    conn.execute("DELETE FROM xllm_group_stability_samples")

    channel_rows = conn.execute('SELECT id, "group", status FROM channels').fetchall()
    channels = {
        int(row[0]): {
            "groups": normalize_groups(row[1]),
            "status": int(row[2] or 0),
        }
        for row in channel_rows
    }

    logs = conn.execute(
        """
        SELECT id, channel_id, created_at, use_time, token_name, content
        FROM logs
        WHERE type=2 AND channel_id IS NOT NULL AND channel_id > 0
        ORDER BY created_at ASC, id ASC
        """,
    ).fetchall()

    samples = []
    current_bucket: list[sqlite3.Row] = []
    last_created_at = 0
    task_index = 0

    def is_model_test(row: sqlite3.Row) -> bool:
        token_name = str(row[4] or "")
        content = str(row[5] or "")
        return token_name == content and token_name != ""

    def flush_bucket(bucket: list[sqlite3.Row]) -> None:
        nonlocal task_index
        if not bucket:
            return
        task_index += 1
        task_id = f"backfill:{task_index:06d}:{int(bucket[0][2])}"
        for row in bucket:
            channel_id = int(row[1] or 0)
            channel = channels.get(channel_id)
            if not channel or not channel["groups"]:
                continue
            tested_at = int(row[2] or 0)
            use_time = int(row[3] or 0)
            response_time_ms = max(use_time, 0) * 1000
            status = int(channel["status"])
            samples.append(
                (
                    task_id,
                    channel_id,
                    ",".join(channel["groups"]),
                    1,
                    "",
                    response_time_ms,
                    status,
                    status,
                    tested_at,
                    int(time.time()),
                ),
            )

    for row in logs:
        if not is_model_test(row):
            continue
        created_at = int(row[2] or 0)
        if not current_bucket:
            current_bucket = [row]
            last_created_at = created_at
            continue
        if created_at - last_created_at <= 5 * 60:
            current_bucket.append(row)
            last_created_at = created_at
            continue
        flush_bucket(current_bucket)
        current_bucket = [row]
        last_created_at = created_at
    flush_bucket(current_bucket)

    if samples:
        conn.executemany(
            """
            INSERT INTO xllm_group_stability_samples
            (task_id, channel_id, group_names, success, error_code, response_time_ms,
             channel_status_before, channel_status_after, tested_at, created_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            samples,
        )
    return len(samples)


def write_group_config(conn: sqlite3.Connection, config_path: Path) -> list[str]:
    available_groups: set[str] = set()
    for (group_value,) in conn.execute('SELECT "group" FROM channels WHERE status=1 ORDER BY priority DESC, id ASC'):
        for group in normalize_groups(group_value):
            if group.startswith("测试组"):
                continue
            available_groups.add(group)

    configured_groups = [group for group in DEFAULT_GROUP_ORDER if group in available_groups]

    config = {
        "enabled": True,
        "groups": [
            {
                "group": group,
                "display_name": group,
                "enabled": True,
                "sort_order": index * 10,
            }
            for index, group in enumerate(configured_groups, start=1)
        ],
    }
    config_path.parent.mkdir(parents=True, exist_ok=True)
    config_path.write_text(json.dumps(config, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return configured_groups


def main() -> None:
    args = parse_args()
    root = repo_root()
    db_path = ensure_local_target(root / args.db)
    config_path = root / "data" / "xllm-group-stability.json"

    counts = fetch_remote_counts(args.ssh_host, args.compose_dir, args.days)
    print("Remote counts:")
    for key, value in counts.items():
        print(f"  {key}: {value}")
    if args.dry_run:
        return

    if db_path.exists():
        backup_path = backup_local_db(db_path)
        print(f"Local DB backup: {backup_path}")
    else:
        db_path.parent.mkdir(parents=True, exist_ok=True)

    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row
    try:
        local_root_password = capture_local_root_password(conn) if args.keep_local_root_password else None
        conn.execute("PRAGMA foreign_keys=OFF")
        conn.execute("BEGIN")

        imported: dict[str, int] = {}
        for table in [*CORE_TABLES, *OPTIONAL_TABLES]:
            columns, rows = fetch_table_rows(args.ssh_host, args.compose_dir, table)
            clear_table(conn, table)
            imported[table] = insert_rows(conn, table, columns, rows)

        log_where = f"created_at >= UNIX_TIMESTAMP(NOW() - INTERVAL {args.days} DAY)"
        columns, rows = fetch_table_rows(args.ssh_host, args.compose_dir, LOG_TABLE, log_where)
        clear_table(conn, LOG_TABLE)
        imported[LOG_TABLE] = insert_rows(conn, LOG_TABLE, columns, rows)

        restore_local_root_password(conn, local_root_password)
        samples = build_group_stability_samples(conn, MODEL_TEST_TOKEN_NAME)
        groups = write_group_config(conn, config_path)

        conn.commit()
    except Exception:
        conn.rollback()
        raise
    finally:
        conn.close()

    print("Imported rows:")
    for key, value in imported.items():
        print(f"  {key}: {value}")
    print(f"Generated xllm_group_stability_samples: {samples}")
    print(f"Wrote config: {config_path}")
    print("Configured groups:")
    for group in groups:
        print(f"  - {group}")


if __name__ == "__main__":
    main()
