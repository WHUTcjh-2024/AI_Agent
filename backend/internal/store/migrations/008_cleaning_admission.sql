-- A legacy ACCEPTED/RAG flag is not a parsing or PII scan receipt.
-- Existing rows remain blocked until a verified cleaning batch is promoted.
ALTER TABLE knowledge.documents
    ADD COLUMN IF NOT EXISTS parse_status TEXT NOT NULL DEFAULT 'PENDING',
    ADD COLUMN IF NOT EXISTS pii_scan_status TEXT NOT NULL DEFAULT 'PENDING',
    ADD COLUMN IF NOT EXISTS pii_content_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS content_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS content_chars INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS secondary_topic TEXT NOT NULL DEFAULT 'other',
    ADD COLUMN IF NOT EXISTS source_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS canonical_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS admission_status TEXT NOT NULL DEFAULT 'BLOCKED',
    ADD COLUMN IF NOT EXISTS admission_version TEXT NOT NULL DEFAULT '';

ALTER TABLE knowledge.attachments
    ADD COLUMN IF NOT EXISTS parse_status TEXT NOT NULL DEFAULT 'PENDING',
    ADD COLUMN IF NOT EXISTS pii_scan_status TEXT NOT NULL DEFAULT 'PENDING',
    ADD COLUMN IF NOT EXISTS pii_content_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS content_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS admission_status TEXT NOT NULL DEFAULT 'BLOCKED',
    ADD COLUMN IF NOT EXISTS relation_status TEXT NOT NULL DEFAULT 'UNRESOLVED';

INSERT INTO schema_migrations(version) VALUES ('008_cleaning_admission') ON CONFLICT DO NOTHING;
