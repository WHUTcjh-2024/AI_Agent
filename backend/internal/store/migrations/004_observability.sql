-- Phase 10A keeps operational queries fast without duplicating AgentRun facts.
CREATE INDEX IF NOT EXISTS users_school_created_idx
    ON users(current_school_id, created_at);
CREATE INDEX IF NOT EXISTS sessions_school_user_idx
    ON sessions(school_id, user_id);
CREATE INDEX IF NOT EXISTS messages_role_created_idx
    ON messages(role, created_at, session_id);
CREATE INDEX IF NOT EXISTS agent_runs_status_created_idx
    ON agent_runs(status, created_at, session_id);
CREATE INDEX IF NOT EXISTS run_events_type_created_idx
    ON run_events(event_type, created_at, run_id);
CREATE INDEX IF NOT EXISTS feedback_value_created_idx
    ON feedback(value, created_at, message_id);
CREATE INDEX IF NOT EXISTS usage_records_created_idx
    ON usage_records(created_at, run_id);

INSERT INTO schema_migrations(version) VALUES ('004_observability') ON CONFLICT DO NOTHING;
