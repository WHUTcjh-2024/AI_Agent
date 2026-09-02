"""PostgreSQL 访问层。

§16：正式状态只放 PostgreSQL，不引入 SQLite 作为状态库。
§35：多 Agent 共享 PostgreSQL 状态，task claim 使用
     SELECT ... FOR UPDATE SKIP LOCKED，保证不重复执行同一 URL。
约束 #1：数据库只允许新增 Migration，禁止 DROP / 重建 / 覆盖现有 knowledge.* 表。
     本模块在应用迁移前会做静态安全检查，任何破坏性语句都会被拒绝执行。
"""

from __future__ import annotations

import os
import re
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional, Sequence

import psycopg
from psycopg import sql
from psycopg.rows import dict_row
from psycopg_pool import ConnectionPool

from .config import MIGRATIONS_DIR, PipelineConfig

DEFAULT_DSN = os.environ.get(
    "ASKU_KNOWLEDGE_DSN",
    "postgresql://asku:asku_dev@127.0.0.1:55432/asku?sslmode=disable",
)

# ---------------------------------------------------------------------------
# 迁移安全网（约束 #1）
# ---------------------------------------------------------------------------

# 任何匹配到这些模式的迁移都会被拒绝，防止误伤现有 knowledge.* 契约
_FORBIDDEN_SQL_PATTERNS = [
    re.compile(r"\bDROP\s+TABLE\b", re.IGNORECASE),
    re.compile(r"\bDROP\s+SCHEMA\b", re.IGNORECASE),
    re.compile(r"\bDROP\s+COLUMN\b", re.IGNORECASE),
    re.compile(r"\bTRUNCATE\b", re.IGNORECASE),
    re.compile(r"\bALTER\s+TABLE\s+[\w.]*\b.*\bDROP\b", re.IGNORECASE | re.DOTALL),
    re.compile(r"\bCREATE\s+OR\s+REPLACE\s+TABLE\b", re.IGNORECASE),
    re.compile(r"\bDELETE\s+FROM\s+knowledge\.", re.IGNORECASE),
    re.compile(r"\bUPDATE\s+knowledge\.\w+\s+SET\b", re.IGNORECASE),
]


class UnsafeMigrationError(RuntimeError):
    """迁移脚本包含破坏性语句。"""


class MigrationDriftError(RuntimeError):
    """已应用迁移的内容被修改。"""


def assert_migration_is_additive(script: str, filename: str) -> None:
    """静态检查：迁移只允许 CREATE IF NOT EXISTS / ADD COLUMN IF NOT EXISTS / INSERT。"""
    # 去掉注释后再检查，避免注释里的示例语句触发误报
    stripped = re.sub(r"--[^\n]*", "", script)
    stripped = re.sub(r"/\*.*?\*/", "", stripped, flags=re.DOTALL)
    for pattern in _FORBIDDEN_SQL_PATTERNS:
        match = pattern.search(stripped)
        if match:
            raise UnsafeMigrationError(
                f"迁移 {filename} 含破坏性语句，已拒绝执行: {match.group(0)!r}"
            )
    allowed = (
        re.compile(r"^CREATE\s+(?:SCHEMA|TABLE|(?:UNIQUE\s+)?INDEX)\s+IF\s+NOT\s+EXISTS\b", re.IGNORECASE | re.DOTALL),
        re.compile(r"^ALTER\s+TABLE\s+[\w.]+\s+ADD\s+COLUMN\s+IF\s+NOT\s+EXISTS\b", re.IGNORECASE | re.DOTALL),
    )
    for statement in _split_statements(stripped):
        if not any(pattern.match(statement.strip()) for pattern in allowed):
            preview = re.sub(r"\s+", " ", statement.strip())[:160]
            raise UnsafeMigrationError(
                f"迁移 {filename} 含非新增语句，已拒绝执行: {preview!r}"
            )


def _split_statements(script: str) -> List[str]:
    """极简 SQL 语句切分：尊重 $$ 包裹的函数体与单引号字符串。"""
    statements: List[str] = []
    buffer: List[str] = []
    in_dollar = False
    in_single = False
    index = 0
    length = len(script)
    while index < length:
        char = script[index]
        if not in_single and script.startswith("$$", index):
            in_dollar = not in_dollar
            buffer.append("$$")
            index += 2
            continue
        if not in_dollar:
            if char == "'":
                # 处理 '' 转义
                if in_single and index + 1 < length and script[index + 1] == "'":
                    buffer.append("''")
                    index += 2
                    continue
                in_single = not in_single
            if not in_single and char == ";":
                statement = "".join(buffer).strip()
                if statement:
                    statements.append(statement)
                buffer = []
                index += 1
                continue
        buffer.append(char)
        index += 1
    tail = "".join(buffer).strip()
    if tail:
        statements.append(tail)
    return statements


# ---------------------------------------------------------------------------
# 连接与迁移
# ---------------------------------------------------------------------------


class Database:
    def __init__(self, dsn: str = DEFAULT_DSN, min_size: int = 1, max_size: int = 8):
        self.dsn = dsn
        self.pool = ConnectionPool(
            conninfo=dsn,
            min_size=min_size,
            max_size=max_size,
            kwargs={"row_factory": dict_row, "autocommit": False},
            open=False,
        )

    def open(self) -> "Database":
        self.pool.open(wait=True, timeout=30)
        return self

    def close(self) -> None:
        self.pool.close()

    @contextmanager
    def connection(self):
        with self.pool.connection() as conn:
            yield conn

    @contextmanager
    def transaction(self):
        with self.pool.connection() as conn:
            with conn.transaction():
                yield conn

    # ---- 迁移 ----

    def ensure_migration_table(self) -> None:
        with self.transaction() as conn:
            conn.execute("CREATE SCHEMA IF NOT EXISTS crawler")
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS crawler.schema_migrations (
                    filename     text PRIMARY KEY,
                    applied_at   timestamptz NOT NULL DEFAULT now(),
                    checksum     text NOT NULL
                )
                """
            )

    def applied_migrations(self) -> Dict[str, str]:
        self.ensure_migration_table()
        with self.connection() as conn:
            rows = conn.execute("SELECT filename,checksum FROM crawler.schema_migrations").fetchall()
        return {row["filename"]: row["checksum"] for row in rows}

    def migrate(self, migrations_dir: Path = MIGRATIONS_DIR, *, dry_run: bool = False) -> List[str]:
        """按文件名顺序应用未执行的迁移。全程 additive，破坏性语句直接抛错。"""
        import hashlib

        applied = self.applied_migrations()
        files = sorted(p for p in migrations_dir.glob("*.sql"))
        executed: List[str] = []
        for path in files:
            script = path.read_text(encoding="utf-8")
            assert_migration_is_additive(script, path.name)
            checksum = hashlib.sha256(script.encode("utf-8")).hexdigest()
            if path.name in applied:
                if applied[path.name] != checksum:
                    raise MigrationDriftError(
                        f"已应用迁移 {path.name} 的校验和发生变化；请新增迁移，不要修改历史文件"
                    )
                continue
            if dry_run:
                executed.append(path.name)
                continue
            statements = _split_statements(script)
            with self.transaction() as conn:
                for statement in statements:
                    conn.execute(statement)
                conn.execute(
                    "INSERT INTO crawler.schema_migrations (filename, checksum) VALUES (%s, %s)",
                    (path.name, checksum),
                )
            executed.append(path.name)
        return executed

    # ---- 通用查询辅助 ----

    def execute(self, query: str, params: Optional[Sequence[Any]] = None) -> int:
        with self.transaction() as conn:
            cur = conn.execute(query, params or ())
            return cur.rowcount

    def fetchone(self, query: str, params: Optional[Sequence[Any]] = None):
        with self.connection() as conn:
            return conn.execute(query, params or ()).fetchone()

    def fetchall(self, query: str, params: Optional[Sequence[Any]] = None):
        with self.connection() as conn:
            return conn.execute(query, params or ()).fetchall()


# ---------------------------------------------------------------------------
# URL 队列（§16 crawler.urls 状态机 + §35 claim）
# ---------------------------------------------------------------------------

URL_STATUSES = (
    "DISCOVERED", "PENDING", "CLAIMED", "FETCHED", "PARSED", "FAILED", "IGNORED",
)


@dataclass
class UrlRecord:
    id: str
    school_id: str
    url: str
    canonical_url: str
    source_id: Optional[str]
    status: str
    priority: int
    depth: int
    discovered_from: str
    http_status: Optional[int] = None
    retry_count: int = 0
    last_error: str = ""
    etag: str = ""
    last_modified: str = ""
    content_hash: str = ""
    claimed_by: str = ""
    claimed_at: Optional[datetime] = None
    last_crawled_at: Optional[datetime] = None
    next_crawl_at: Optional[datetime] = None


class UrlRepository:
    """crawler.urls 的仓储：入队、认领、回写状态。"""

    def __init__(self, db: Database, agent_id: str, lease_seconds: int = 900):
        self.db = db
        self.agent_id = agent_id
        self.lease_seconds = lease_seconds

    # ---- 入队 ----

    def enqueue(
        self,
        *,
        school_id: str,
        url: str,
        canonical_url: str,
        source_id: Optional[str] = None,
        priority: int = 5,
        depth: int = 0,
        discovered_from: str = "",
    ) -> Optional[str]:
        """幂等入队：同一 canonical_url 重复入队不会产生重复行。"""
        row = self.db.fetchone(
            """
            INSERT INTO crawler.urls
                (school_id, url, canonical_url, source_id, status, priority, depth, discovered_from)
            VALUES (%s, %s, %s, %s, 'DISCOVERED', %s, %s, %s)
            ON CONFLICT (school_id, canonical_url) DO NOTHING
            RETURNING id
            """,
            (school_id, url, canonical_url, source_id, priority, depth, discovered_from),
        )
        return row["id"] if row else None

    def enqueue_many(self, rows: List[Dict[str, Any]]) -> int:
        """批量入队，返回实际新增行数。"""
        if not rows:
            return 0
        inserted = 0
        with self.db.transaction() as conn:
            for row in rows:
                cur = conn.execute(
                    """
                    INSERT INTO crawler.urls
                        (school_id, url, canonical_url, source_id, status, priority, depth, discovered_from)
                    VALUES (%s, %s, %s, %s, 'DISCOVERED', %s, %s, %s)
                    ON CONFLICT (school_id, canonical_url) DO NOTHING
                    """,
                    (
                        row["school_id"], row["url"], row["canonical_url"],
                        row.get("source_id"), row.get("priority", 5),
                        row.get("depth", 0), row.get("discovered_from", ""),
                    ),
                )
                inserted += cur.rowcount
        return inserted

    # ---- 认领（§35） ----

    def claim_batch(self, limit: int = 20, *, school_id: str = "whut") -> List[UrlRecord]:
        """使用 FOR UPDATE SKIP LOCKED 认领一批 URL，保证多 Agent 不重复。

        同时回收租约过期（agent 崩溃）的 CLAIMED 记录，实现 Crash Recovery。
        """
        with self.db.transaction() as conn:
            conn.execute(
                """
                UPDATE crawler.urls
                   SET status='PENDING', claimed_by=NULL, claimed_at=NULL
                 WHERE status='CLAIMED'
                   AND claimed_at < now() - (%s * interval '1 second')
                """,
                (self.lease_seconds,),
            )
            rows = conn.execute(
                """
                UPDATE crawler.urls AS u
                   SET status='CLAIMED', claimed_by=%s, claimed_at=now(), updated_at=now()
                  FROM (
                        SELECT id FROM crawler.urls
                         WHERE status IN ('DISCOVERED','PENDING')
                           AND school_id = %s
                           AND (next_crawl_at IS NULL OR next_crawl_at <= now())
                         ORDER BY priority ASC, depth ASC, created_at ASC
                         LIMIT %s
                         FOR UPDATE SKIP LOCKED
                       ) AS picked
                 WHERE u.id = picked.id
                RETURNING u.*
                """,
                (self.agent_id, school_id, limit),
            ).fetchall()
        return [UrlRecord(**{k: row.get(k) for k in UrlRecord.__dataclass_fields__}) for row in rows]

    # ---- 状态回写 ----

    def mark_fetched(
        self,
        url_id: str,
        *,
        http_status: int,
        content_hash: str = "",
        etag: str = "",
        last_modified: str = "",
        raw_path: str = "",
    ) -> None:
        self.db.execute(
            """
            UPDATE crawler.urls
               SET status='FETCHED', http_status=%s, content_hash=%s, etag=%s,
                   last_modified=%s, raw_path=%s, last_crawled_at=now(),
                   claimed_by=NULL, claimed_at=NULL, updated_at=now()
             WHERE id=%s
            """,
            (http_status, content_hash, etag, last_modified, raw_path, url_id),
        )

    def mark_parsed(self, url_id: str) -> None:
        self.db.execute(
            """
            UPDATE crawler.urls
               SET status='PARSED', updated_at=now(),
                   next_crawl_at=now() + interval '7 days'
             WHERE id=%s
            """,
            (url_id,),
        )

    def mark_failed(self, url_id: str, error: str, *, http_status: Optional[int] = None) -> None:
        self.db.execute(
            """
            UPDATE crawler.urls
               SET status=CASE WHEN retry_count + 1 >= 3 THEN 'FAILED' ELSE 'PENDING' END,
                   retry_count = retry_count + 1,
                   last_error = %s,
                   http_status = COALESCE(%s, http_status),
                   claimed_by=NULL, claimed_at=NULL,
                   next_crawl_at = now() + (power(2, LEAST(retry_count, 5)) * interval '1 minute'),
                   updated_at=now()
             WHERE id=%s
            """,
            (error[:2000], http_status, url_id),
        )

    def mark_ignored(self, url_id: str, reason: str) -> None:
        self.db.execute(
            """
            UPDATE crawler.urls
               SET status='IGNORED', last_error=%s, claimed_by=NULL, claimed_at=NULL, updated_at=now()
             WHERE id=%s
            """,
            (reason[:500], url_id),
        )

    def record_attempt(
        self,
        url_id: str,
        *,
        attempt: int,
        http_status: Optional[int],
        error_type: str,
        error_message: str,
        started_at: datetime,
        finished_at: datetime,
    ) -> None:
        self.db.execute(
            """
            INSERT INTO crawler.crawl_attempts
                (url_id, attempt, started_at, finished_at, http_status, error_type, error_message, agent_id)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
            """,
            (
                url_id, attempt, started_at, finished_at, http_status,
                error_type, (error_message or "")[:2000], self.agent_id,
            ),
        )

    # ---- 统计 ----

    def counts_by_status(self, school_id: str = "whut") -> Dict[str, int]:
        rows = self.db.fetchall(
            """
            SELECT status, count(*) AS total
              FROM crawler.urls
             WHERE school_id=%s
             GROUP BY status
            """,
            (school_id,),
        )
        return {row["status"]: row["total"] for row in rows}

    def processed_count(self, school_id: str = "whut") -> int:
        row = self.db.fetchone(
            """
            SELECT count(*) AS total FROM crawler.urls
             WHERE school_id=%s AND status IN ('FETCHED','PARSED','FAILED','IGNORED')
            """,
            (school_id,),
        )
        return int(row["total"]) if row else 0

    def source_processed_count(self, source_id: str) -> int:
        row = self.db.fetchone(
            """
            SELECT count(*) AS total FROM crawler.urls
             WHERE source_id=%s AND status IN ('FETCHED','PARSED')
            """,
            (source_id,),
        )
        return int(row["total"]) if row else 0


def open_database(config: Optional[PipelineConfig] = None, dsn: Optional[str] = None) -> Database:
    """便捷入口：优先使用环境变量中的 DSN。"""
    return Database(dsn=dsn or DEFAULT_DSN).open()


def utcnow() -> datetime:
    return datetime.now(timezone.utc)
