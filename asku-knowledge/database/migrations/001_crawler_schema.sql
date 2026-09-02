-- 001: crawler schema 与 URL 状态机
--
-- 约束 #1：本迁移纯新增。只 CREATE SCHEMA IF NOT EXISTS / CREATE TABLE IF NOT EXISTS /
--          CREATE INDEX IF NOT EXISTS。不触碰任何现有 knowledge.* 表。
--
-- §16：crawler.urls / crawler.crawl_attempts 字段清单。
-- §35：claimed_by / claimed_at 支持 Agent Claim / Lease / Crash Recovery。

CREATE SCHEMA IF NOT EXISTS crawler;

-- ---------------------------------------------------------------------------
-- crawler.urls —— URL 队列与状态机
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS crawler.urls (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         text        NOT NULL,
    url               text        NOT NULL,
    canonical_url     text        NOT NULL,
    source_id         text,
    source_key        text        NOT NULL DEFAULT '',
    status            text        NOT NULL DEFAULT 'DISCOVERED',
    priority          integer     NOT NULL DEFAULT 5,
    depth             integer     NOT NULL DEFAULT 0,
    discovered_from   text        NOT NULL DEFAULT '',
    http_status       integer,
    retry_count       integer     NOT NULL DEFAULT 0,
    last_error        text        NOT NULL DEFAULT '',
    etag              text        NOT NULL DEFAULT '',
    last_modified     text        NOT NULL DEFAULT '',
    content_hash      text        NOT NULL DEFAULT '',
    raw_path          text        NOT NULL DEFAULT '',
    normalized_path   text        NOT NULL DEFAULT '',
    claimed_by        text,
    claimed_at        timestamptz,
    last_crawled_at   timestamptz,
    next_crawl_at     timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT crawler_urls_status_chk CHECK (
        status IN ('DISCOVERED','PENDING','CLAIMED','FETCHED','PARSED','FAILED','IGNORED')
    )
);

-- 幂等入队依赖的唯一键：同一学校下同一 canonical_url 只存在一行
CREATE UNIQUE INDEX IF NOT EXISTS crawler_urls_school_canonical_uniq
    ON crawler.urls (school_id, canonical_url);

-- 认领扫描路径：状态 + 优先级 + 深度
CREATE INDEX IF NOT EXISTS crawler_urls_claim_idx
    ON crawler.urls (school_id, status, priority, depth, created_at)
    WHERE status IN ('DISCOVERED','PENDING');

-- 租约回收扫描
CREATE INDEX IF NOT EXISTS crawler_urls_lease_idx
    ON crawler.urls (claimed_at) WHERE status = 'CLAIMED';

CREATE INDEX IF NOT EXISTS crawler_urls_source_idx
    ON crawler.urls (source_id, status);

-- 内容去重：同哈希只保留一条（§21 Content Hash）
CREATE INDEX IF NOT EXISTS crawler_urls_content_hash_idx
    ON crawler.urls (school_id, content_hash) WHERE content_hash <> '';

-- ---------------------------------------------------------------------------
-- crawler.crawl_attempts —— 每次抓取尝试的审计记录
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS crawler.crawl_attempts (
    id             bigserial PRIMARY KEY,
    url_id         uuid       NOT NULL,
    attempt        integer    NOT NULL,
    started_at     timestamptz NOT NULL,
    finished_at    timestamptz,
    http_status    integer,
    error_type     text       NOT NULL DEFAULT '',
    error_message  text       NOT NULL DEFAULT '',
    agent_id       text       NOT NULL DEFAULT '',
    duration_ms    integer,
    bytes_downloaded bigint,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS crawler_attempts_url_idx
    ON crawler.crawl_attempts (url_id, attempt);

CREATE INDEX IF NOT EXISTS crawler_attempts_error_idx
    ON crawler.crawl_attempts (error_type) WHERE error_type <> '';

-- ---------------------------------------------------------------------------
-- crawler.checkpoints —— 长任务断点续跑（约束 #4 达预算即 checkpoint 后退出）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS crawler.checkpoints (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id      text        NOT NULL,
    agent_id       text        NOT NULL,
    phase          text        NOT NULL DEFAULT '',
    reason         text        NOT NULL DEFAULT '',
    urls_processed integer     NOT NULL DEFAULT 0,
    documents_seen integer     NOT NULL DEFAULT 0,
    state          jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS crawler_checkpoints_school_idx
    ON crawler.checkpoints (school_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- crawler.robots_cache —— robots.txt 缓存，避免重复请求
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS crawler.robots_cache (
    host           text PRIMARY KEY,
    content        text NOT NULL DEFAULT '',
    fetched_at     timestamptz NOT NULL DEFAULT now(),
    allows_default boolean NOT NULL DEFAULT true
);
