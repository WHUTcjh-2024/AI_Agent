-- Additive promotion contract for independently verified cleaning batches.
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS parse_status text NOT NULL DEFAULT 'PENDING';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS parser_version text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS parse_errors jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS pii_scan_status text NOT NULL DEFAULT 'PENDING';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS pii_content_hash text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS pii_rule_version text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS relation_status text NOT NULL DEFAULT 'UNRESOLVED';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS admission_status text NOT NULL DEFAULT 'BLOCKED';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS admission_reasons jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS admission_version text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS normalized_sha256 text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS raw_sha256 text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS parse_format text NOT NULL DEFAULT '';
ALTER TABLE knowledge.documents ADD COLUMN IF NOT EXISTS publish_date_evidence text NOT NULL DEFAULT '';

ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS parse_status text NOT NULL DEFAULT 'PENDING';
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS pii_scan_status text NOT NULL DEFAULT 'PENDING';
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS pii_content_hash text NOT NULL DEFAULT '';
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS content_hash text NOT NULL DEFAULT '';
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS admission_status text NOT NULL DEFAULT 'BLOCKED';
ALTER TABLE knowledge.attachments ADD COLUMN IF NOT EXISTS relation_status text NOT NULL DEFAULT 'UNRESOLVED';
