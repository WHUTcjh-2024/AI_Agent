-- 003: 新增知识组织与治理表
--
-- 约束 #1：纯新增。只创建当前不存在的新表。
--
-- 约束 #3：knowledge.candidate_sources 用于记录站外链接作为候选来源，
--         记录后不自动抓取，需人工或后续决策批准后才进入 SourceRegistry。
-- 约束 #5：knowledge.pii_reviews 记录命中 PII 的文档，一律 PII_REVIEW + NO_RAG。

-- ---------------------------------------------------------------------------
-- knowledge.knowledge_bundles —— 网页与其附件的整体（§13）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS knowledge.knowledge_bundles (
    id                   text PRIMARY KEY,
    school_id            text NOT NULL,
    topic                text NOT NULL DEFAULT '',
    title                text NOT NULL DEFAULT '',
    primary_document_id  text,
    document_count       integer NOT NULL DEFAULT 0,
    attachment_count     integer NOT NULL DEFAULT 0,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS knowledge_bundles_school_idx
    ON knowledge.knowledge_bundles (school_id, topic);

-- ---------------------------------------------------------------------------
-- knowledge.document_relations —— 版本与文档关系（§10）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS knowledge.document_relations (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         text NOT NULL,
    from_document_id  text NOT NULL,
    to_document_id    text NOT NULL,
    relation_type     text NOT NULL,
    confidence        numeric(4,3) NOT NULL DEFAULT 0,
    evidence          text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT knowledge_document_relations_type_chk CHECK (
        relation_type IN (
            'SUPERSEDES','IMPLEMENTATION_OF','SUPPLEMENT_OF',
            'ATTACHMENT_OF','MIRROR_OF','RELATED_TO'
        )
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS knowledge_document_relations_uniq
    ON knowledge.document_relations (from_document_id, to_document_id, relation_type);

CREATE INDEX IF NOT EXISTS knowledge_document_relations_from_idx
    ON knowledge.document_relations (from_document_id);

-- ---------------------------------------------------------------------------
-- knowledge.candidate_sources —— 站外候选来源（只记录，不自动抓取）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS knowledge.candidate_sources (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id        text NOT NULL,
    url              text NOT NULL,
    canonical_url    text NOT NULL,
    host             text NOT NULL DEFAULT '',
    found_from_url   text NOT NULL DEFAULT '',
    anchor_text      text NOT NULL DEFAULT '',
    link_count       integer NOT NULL DEFAULT 1,
    decision         text NOT NULL DEFAULT 'PENDING',
    note             text NOT NULL DEFAULT '',
    first_seen_at    timestamptz NOT NULL DEFAULT now(),
    last_seen_at     timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS knowledge_candidate_sources_uniq
    ON knowledge.candidate_sources (school_id, canonical_url);

CREATE INDEX IF NOT EXISTS knowledge_candidate_sources_host_idx
    ON knowledge.candidate_sources (school_id, host, link_count DESC);

-- ---------------------------------------------------------------------------
-- knowledge.pii_reviews —— PII 复核队列（约束 #5）
-- 命中学生名单 / 学号 / 手机号 / 身份证 / 个人成绩等一律入队，
-- 同时把文档置为 PII_REVIEW + rag_eligible=false，禁止进入 WeKnora。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS knowledge.pii_reviews (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id     text NOT NULL,
    document_id   text,
    attachment_id text,
    url           text NOT NULL DEFAULT '',
    categories    jsonb NOT NULL DEFAULT '[]'::jsonb,
    match_count   integer NOT NULL DEFAULT 0,
    snippet       text NOT NULL DEFAULT '',
    status        text NOT NULL DEFAULT 'PII_REVIEW',
    resolved_at   timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS knowledge_pii_reviews_status_idx
    ON knowledge.pii_reviews (school_id, status, created_at DESC);

-- ---------------------------------------------------------------------------
-- knowledge.coverage_runs —— Golden Question 覆盖审计轮次历史（§29、约束 #7）
-- 连续两轮无实质提升即停止自动扩展，等待下一轮针对性采集。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS knowledge.coverage_runs (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id          text NOT NULL,
    round_no           integer NOT NULL,
    total_questions    integer NOT NULL DEFAULT 0,
    supported          integer NOT NULL DEFAULT 0,
    partial            integer NOT NULL DEFAULT 0,
    zero_result        integer NOT NULL DEFAULT 0,
    coverage_pct       numeric(5,2) NOT NULL DEFAULT 0,
    improvement_pct    numeric(5,2) NOT NULL DEFAULT 0,
    stalled            boolean NOT NULL DEFAULT false,
    gaps_path          text NOT NULL DEFAULT '',
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS knowledge_coverage_runs_school_idx
    ON knowledge.coverage_runs (school_id, round_no DESC);

-- ---------------------------------------------------------------------------
-- knowledge.coverage_results —— 每题的覆盖判定明细（§30）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS knowledge.coverage_results (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id         uuid NOT NULL,
    question_id    text NOT NULL,
    question       text NOT NULL DEFAULT '',
    verdict        text NOT NULL,
    hit_count      integer NOT NULL DEFAULT 0,
    top_score      numeric(8,5) NOT NULL DEFAULT 0,
    best_document_id text NOT NULL DEFAULT '',
    note           text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT knowledge_coverage_results_verdict_chk CHECK (
        verdict IN (
            'SUPPORTED','PARTIAL','ZERO_RESULT','WRONG_SOURCE',
            'OUTDATED_SOURCE','CONFLICTING_SOURCE','UNANSWERABLE_BY_DESIGN'
        )
    )
);

CREATE INDEX IF NOT EXISTS knowledge_coverage_results_run_idx
    ON knowledge.coverage_results (run_id);

CREATE INDEX IF NOT EXISTS knowledge_coverage_results_question_idx
    ON knowledge.coverage_results (question_id, verdict);

-- ---------------------------------------------------------------------------
-- knowledge.staging_documents —— 批量数据入库前的暂存区（约束 #2）
-- 在 Canary Import 与人工抽样通过前，数据只停留在 raw / normalized / JSONL / staging，
-- 不进入 WeKnora 全量导入。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS knowledge.staging_documents (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       text NOT NULL,
    document_id     text NOT NULL,
    payload         jsonb NOT NULL,
    source_url      text NOT NULL DEFAULT '',
    file_hash       text NOT NULL DEFAULT '',
    gate_status     text NOT NULL DEFAULT 'STAGED',
    gate_reason     text NOT NULL DEFAULT '',
    promoted_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS knowledge_staging_documents_uniq
    ON knowledge.staging_documents (school_id, document_id, file_hash);

CREATE INDEX IF NOT EXISTS knowledge_staging_documents_gate_idx
    ON knowledge.staging_documents (school_id, gate_status);
