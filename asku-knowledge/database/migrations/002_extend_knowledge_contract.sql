-- 002: 为现有 knowledge.* 契约表补齐文档要求的字段
--
-- 约束 #1：纯 additive。全部使用 ADD COLUMN IF NOT EXISTS，
--         不删除列、不修改列类型、不重建表、不覆盖数据。
--         现有后端 SELECT 依赖的列（source_name / authority / official_url /
--         parent_page_url / attachment_original_url / freshness / local_file_path）
--         一律保持原样，保证 backend/internal/store/postgres.go 的查询不受影响。
--
-- 约束 #6：official_url、attachment_original_url、parent_page_url 必须保留；
--         local_file_path 永远只作为内部存储路径，不作为公开 URL 暴露。

-- ---------------------------------------------------------------------------
-- knowledge.sources
-- ---------------------------------------------------------------------------
ALTER TABLE knowledge.sources ADD COLUMN IF NOT EXISTS source_key        text NOT NULL DEFAULT '';
ALTER TABLE knowledge.sources ADD COLUMN IF NOT EXISTS base_url          text NOT NULL DEFAULT '';
ALTER TABLE knowledge.sources ADD COLUMN IF NOT EXISTS domains           jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE knowledge.sources ADD COLUMN IF NOT EXISTS authority_type    text NOT NULL DEFAULT 'OFFICIAL_DEPARTMENT';
ALTER TABLE knowledge.sources ADD COLUMN IF NOT EXISTS priority          text NOT NULL DEFAULT 'P2';
ALTER TABLE knowledge.sources ADD COLUMN IF NOT EXISTS education_level   text NOT NULL DEFAULT 'UNKNOWN';
ALTER TABLE knowledge.sources ADD COLUMN IF NOT EXISTS active            boolean NOT NULL DEFAULT true;
ALTER TABLE knowledge.sources ADD COLUMN IF NOT EXISTS name_confirmed    boolean NOT NULL DEFAULT false;
ALTER TABLE knowledge.sources ADD COLUMN IF NOT EXISTS discovered_from   text NOT NULL DEFAULT '';
ALTER TABLE knowledge.sources ADD COLUMN IF NOT EXISTS last_checked_at   timestamptz;

CREATE UNIQUE INDEX IF NOT EXISTS knowledge_sources_school_key_uniq
    ON knowledge.sources (school_id, source_key) WHERE source_key <> '';

-- ---------------------------------------------------------------------------
-- knowledge.documents
-- ---------------------------------------------------------------------------
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS education_level          text NOT NULL DEFAULT 'UNKNOWN';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS primary_module           text NOT NULL DEFAULT 'OTHER';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS secondary_topic          text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS audience                 text NOT NULL DEFAULT 'UNKNOWN';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS source_url               text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS canonical_url            text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS effective_date           date;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS valid_from               date;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS valid_to                 date;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS deadline                 date;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS academic_year            text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS semester                 text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS freshness_type           text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS topic_family             text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS version_family           text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS is_current               boolean;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS current_confidence       numeric(4,3);
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS source_authority         text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS authority_score          numeric(5,2) NOT NULL DEFAULT 0;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS quality_score            integer NOT NULL DEFAULT 0;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS quality_band             text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS classification_confidence numeric(4,3) NOT NULL DEFAULT 0;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS raw_path                 text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS normalized_path          text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS mime_type                text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS file_hash                text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS content_hash             text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS simhash                  text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS rag_eligible             boolean NOT NULL DEFAULT false;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS review_status            text NOT NULL DEFAULT 'PENDING';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS rejection_reason         text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS canonical_document_id    text;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS mirror_urls              jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS pii_detected             boolean NOT NULL DEFAULT false;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS pii_categories           jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS attachment_count         integer NOT NULL DEFAULT 0;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS url_id                   uuid;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS depth                    integer NOT NULL DEFAULT 0;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS content_chars            integer NOT NULL DEFAULT 0;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS is_attachment            boolean NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS knowledge_documents_topic_idx
    ON knowledge.documents (school_id, secondary_topic);
CREATE INDEX IF NOT EXISTS knowledge_documents_module_idx
    ON knowledge.documents (school_id, primary_module);
CREATE INDEX IF NOT EXISTS knowledge_documents_rag_idx
    ON knowledge.documents (school_id, rag_eligible, quality_score DESC);
CREATE INDEX IF NOT EXISTS knowledge_documents_type_idx
    ON knowledge.documents (school_id, document_type);
CREATE INDEX IF NOT EXISTS knowledge_documents_hash_idx
    ON knowledge.documents (school_id, file_hash) WHERE file_hash <> '';
CREATE INDEX IF NOT EXISTS knowledge_documents_current_idx
    ON knowledge.documents (school_id, topic_family, is_current);

-- ---------------------------------------------------------------------------
-- knowledge.attachments
-- ---------------------------------------------------------------------------
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS parent_document_id text;
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS knowledge_bundle_id text NOT NULL DEFAULT '';
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS filename          text NOT NULL DEFAULT '';
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS mime_type         text NOT NULL DEFAULT '';
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS file_path         text NOT NULL DEFAULT '';
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS file_hash         text NOT NULL DEFAULT '';
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS file_size         bigint NOT NULL DEFAULT 0;
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS attachment_role   text NOT NULL DEFAULT 'OTHER';
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS source_url        text NOT NULL DEFAULT '';
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS rag_eligible     boolean NOT NULL DEFAULT false;
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS review_status    text NOT NULL DEFAULT 'PENDING';
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS pii_detected     boolean NOT NULL DEFAULT false;
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS updated_at       timestamptz NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS knowledge_attachments_bundle_idx
    ON knowledge.attachments (knowledge_bundle_id) WHERE knowledge_bundle_id <> '';
CREATE INDEX IF NOT EXISTS knowledge_attachments_hash_idx
    ON knowledge.attachments (file_hash) WHERE file_hash <> '';

-- ---------------------------------------------------------------------------
-- knowledge.weknora_mappings
-- ---------------------------------------------------------------------------
ALTER TABLE knowledge.weknora_mappings ADD COLUMN IF NOT EXISTS weknora_knowledge_base_id text NOT NULL DEFAULT '';
ALTER TABLE knowledge.weknora_mappings ADD COLUMN IF NOT EXISTS import_status            text NOT NULL DEFAULT 'PENDING';
ALTER TABLE knowledge.weknora_mappings ADD COLUMN IF NOT EXISTS imported_at              timestamptz;
ALTER TABLE knowledge.weknora_mappings ADD COLUMN IF NOT EXISTS last_sync_at             timestamptz;
ALTER TABLE knowledge.weknora_mappings ADD COLUMN IF NOT EXISTS file_hash                text NOT NULL DEFAULT '';
ALTER TABLE knowledge.weknora_mappings ADD COLUMN IF NOT EXISTS created_at               timestamptz NOT NULL DEFAULT now();
ALTER TABLE knowledge.weknora_mappings ADD COLUMN IF NOT EXISTS last_error               text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS knowledge_weknora_mappings_status_idx
    ON knowledge.weknora_mappings (school_id, import_status);
CREATE INDEX IF NOT EXISTS knowledge_weknora_mappings_doc_idx
    ON knowledge.weknora_mappings (asku_document_id);
