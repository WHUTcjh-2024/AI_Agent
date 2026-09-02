-- Runtime minimum for enforcing crawler-owned admission decisions.
-- Defaults are intentionally deny-by-default: existing rows must be explicitly
-- approved/imported before they can ground an answer.
ALTER TABLE knowledge.sources
    ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE knowledge.documents
    ADD COLUMN IF NOT EXISTS rag_eligible BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS pii_detected BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS review_status TEXT NOT NULL DEFAULT 'PENDING';

ALTER TABLE knowledge.attachments
    ADD COLUMN IF NOT EXISTS rag_eligible BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS pii_detected BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS review_status TEXT NOT NULL DEFAULT 'PENDING';

ALTER TABLE knowledge.weknora_mappings
    ADD COLUMN IF NOT EXISTS import_status TEXT NOT NULL DEFAULT 'PENDING';

CREATE INDEX IF NOT EXISTS knowledge_runtime_documents_admission_idx
    ON knowledge.documents (school_id, rag_eligible, pii_detected);
CREATE INDEX IF NOT EXISTS knowledge_runtime_mappings_status_idx
    ON knowledge.weknora_mappings (school_id, import_status);

INSERT INTO schema_migrations(version) VALUES ('005_knowledge_admission') ON CONFLICT DO NOTHING;
