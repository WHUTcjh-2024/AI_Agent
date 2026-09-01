CREATE TABLE IF NOT EXISTS usage_records (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    run_id TEXT REFERENCES agent_runs(id) ON DELETE SET NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    input_tokens INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    estimated_cost_micro_rmb BIGINT NOT NULL DEFAULT 0 CHECK (estimated_cost_micro_rmb >= 0),
    latency_ms BIGINT NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
    status TEXT NOT NULL CHECK (status IN ('succeeded', 'failed', 'cancelled')),
    error_code TEXT NOT NULL DEFAULT '',
    tokens_estimated BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS usage_records_user_created_idx
    ON usage_records(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS usage_records_provider_created_idx
    ON usage_records(provider, created_at DESC);
CREATE INDEX IF NOT EXISTS usage_records_run_idx
    ON usage_records(run_id) WHERE run_id IS NOT NULL;

INSERT INTO schema_migrations(version) VALUES ('002_llm_usage') ON CONFLICT DO NOTHING;
