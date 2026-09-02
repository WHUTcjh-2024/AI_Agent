ALTER TABLE sources
    ADD COLUMN IF NOT EXISTS department TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS official_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attachment_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS parent_page_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS document_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS authority TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS freshness TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS knowledge_bundle_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attachments JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS evidence JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE TABLE IF NOT EXISTS message_citations (
    message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    citation_id TEXT NOT NULL,
    citation_index INTEGER NOT NULL CHECK (citation_index > 0),
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    asku_document_id TEXT NOT NULL DEFAULT '',
    weknora_knowledge_id TEXT NOT NULL DEFAULT '',
    chunk_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    source_name TEXT NOT NULL DEFAULT '',
    department TEXT NOT NULL DEFAULT '',
    publish_date TIMESTAMPTZ NOT NULL,
    source_type TEXT NOT NULL DEFAULT '',
    document_type TEXT NOT NULL DEFAULT '',
    official_url TEXT NOT NULL DEFAULT '',
    attachment_url TEXT NOT NULL DEFAULT '',
    parent_page_url TEXT NOT NULL DEFAULT '',
    evidence_text TEXT NOT NULL,
    authority TEXT NOT NULL,
    freshness TEXT NOT NULL DEFAULT '',
    knowledge_bundle_id TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (message_id, citation_index),
    UNIQUE (message_id, citation_id)
);
CREATE INDEX IF NOT EXISTS message_citations_source_idx ON message_citations(source_id);

-- The crawler owns these records. WeKnora only owns parsing and retrieval IDs.
CREATE SCHEMA IF NOT EXISTS knowledge;
CREATE TABLE IF NOT EXISTS knowledge.sources (
    id TEXT PRIMARY KEY,
    school_id TEXT NOT NULL,
    source_name TEXT NOT NULL,
    department TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT '',
    authority TEXT NOT NULL DEFAULT 'OFFICIAL_DEPARTMENT',
    official_url TEXT NOT NULL DEFAULT '',
    canonical_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS knowledge.documents (
    id TEXT PRIMARY KEY,
    school_id TEXT NOT NULL,
    source_id TEXT NOT NULL REFERENCES knowledge.sources(id) ON DELETE RESTRICT,
    title TEXT NOT NULL,
    publish_date TIMESTAMPTZ,
    document_type TEXT NOT NULL DEFAULT '',
    parent_page_url TEXT NOT NULL DEFAULT '',
    knowledge_bundle_id TEXT NOT NULL DEFAULT '',
    freshness TEXT NOT NULL DEFAULT '',
    local_file_path TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS knowledge.attachments (
    id TEXT PRIMARY KEY,
    document_id TEXT NOT NULL REFERENCES knowledge.documents(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    document_type TEXT NOT NULL DEFAULT '',
    attachment_original_url TEXT NOT NULL DEFAULT '',
    parent_page_url TEXT NOT NULL DEFAULT '',
    local_file_path TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS knowledge.weknora_mappings (
    school_id TEXT NOT NULL,
    weknora_knowledge_id TEXT NOT NULL,
    asku_document_id TEXT NOT NULL REFERENCES knowledge.documents(id) ON DELETE CASCADE,
    attachment_id TEXT REFERENCES knowledge.attachments(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (school_id, weknora_knowledge_id)
);

INSERT INTO schema_migrations(version) VALUES ('003_citations') ON CONFLICT DO NOTHING;
