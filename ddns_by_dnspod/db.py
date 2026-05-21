"""SQLite 数据库操作"""

from __future__ import annotations

import sqlite3
from datetime import datetime, timezone
from typing import Optional


def init_db(db_path: str) -> sqlite3.Connection:
    """初始化数据库连接并创建所需的表。"""
    conn = sqlite3.connect(db_path)
    conn.execute("""
        CREATE TABLE IF NOT EXISTS ddns_history (
            id          INTEGER PRIMARY KEY AUTOINCREMENT,
            domain      TEXT    NOT NULL,
            sub_domain  TEXT    NOT NULL,
            record_type TEXT    NOT NULL,
            record_id   INTEGER,
            old_ip      TEXT,
            new_ip      TEXT,
            duration    INTEGER,
            status      TEXT    NOT NULL,
            message     TEXT,
            created_at  TEXT    NOT NULL
        )
    """)
    conn.execute("""
        CREATE TABLE IF NOT EXISTS ddns_state (
            key         TEXT PRIMARY KEY,
            value       TEXT NOT NULL,
            updated_at  TEXT NOT NULL
        )
    """)
    conn.commit()
    return conn


def _get_state(conn: sqlite3.Connection, key: str) -> Optional[str]:
    """读取状态值。"""
    row = conn.execute("SELECT value FROM ddns_state WHERE key = ?", (key,)).fetchone()
    return row[0] if row else None


def _set_state(conn: sqlite3.Connection, key: str, value: str) -> None:
    """写入或更新状态值。"""
    now = datetime.now(timezone.utc).isoformat()
    conn.execute(
        """INSERT INTO ddns_state (key, value, updated_at) VALUES (?, ?, ?)
           ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at""",
        (key, value, now),
    )
    conn.commit()


def get_last_public_ip(conn: sqlite3.Connection) -> Optional[str]:
    """获取上次记录的公网 IP。"""
    return _get_state(conn, "last_public_ip")


def set_last_public_ip(conn: sqlite3.Connection, ip: str) -> None:
    """保存本次公网 IP。"""
    _set_state(conn, "last_public_ip", ip)


def get_last_record_time(
    conn: sqlite3.Connection,
    domain: str,
    sub_domain: str,
    record_type: str,
) -> Optional[float]:
    """查询指定记录最近一次操作的 UTC 时间戳（秒），用于计算持续时间。"""
    row = conn.execute(
        """SELECT created_at FROM ddns_history
           WHERE domain = ? AND sub_domain = ? AND record_type = ?
           ORDER BY id DESC LIMIT 1""",
        (domain, sub_domain, record_type),
    ).fetchone()
    if row:
        try:
            dt = datetime.fromisoformat(row[0])
            return dt.timestamp()
        except (ValueError, OSError):
            return None
    return None


def insert_record(
    conn: sqlite3.Connection,
    domain: str,
    sub_domain: str,
    record_type: str,
    record_id: int | None,
    old_ip: str | None,
    new_ip: str | None,
    status: str,
    message: str | None = None,
    duration: int | None = None,
) -> None:
    """插入一条更新记录。"""
    now = datetime.now(timezone.utc).isoformat()
    conn.execute(
        """INSERT INTO ddns_history
           (domain, sub_domain, record_type, record_id, old_ip, new_ip, duration, status, message, created_at)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
        (
            domain,
            sub_domain,
            record_type,
            record_id,
            old_ip,
            new_ip,
            duration,
            status,
            message,
            now,
        ),
    )
    conn.commit()
