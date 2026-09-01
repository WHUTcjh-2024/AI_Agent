package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"asku/backend/internal/domain"
	"asku/backend/internal/id"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

var ErrNotFound = domain.ErrNotFound

type Postgres struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Postgres, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	config.MaxConns = 12
	config.MinConns = 1
	config.MaxConnLifetime = 30 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Close() { p.pool.Close() }

func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

// RecoverInterruptedRuns closes runs whose in-process worker disappeared after
// a service restart. Durable workers can replace this policy in a later phase.
func (p *Postgres) RecoverInterruptedRuns(ctx context.Context) (int, error) {
	rows, err := p.pool.Query(ctx, `
		WITH interrupted AS (
			UPDATE agent_runs
			SET status='FAILED',error_code='server_restarted',updated_at=now()
			WHERE status NOT IN ('COMPLETED','FAILED','CANCELLED')
			RETURNING id
		)
		INSERT INTO run_events(run_id,event_type,payload)
		SELECT id,'run.failed',jsonb_build_object(
			'error','服务已重启，请重新提问。',
			'retryable',true,
			'code','server_restarted'
		) FROM interrupted
		RETURNING run_id
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var runID string
		if err := rows.Scan(&runID); err != nil {
			return 0, err
		}
		count++
	}
	return count, rows.Err()
}

func (p *Postgres) Migrate(ctx context.Context) error {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if _, err := p.pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func (p *Postgres) UpsertDevUser(ctx context.Context, externalID, nickname, schoolID string) (domain.User, error) {
	user := domain.User{ID: id.New("usr"), Nickname: nickname, SchoolID: schoolID}
	err := p.pool.QueryRow(ctx, `
		INSERT INTO users(id, wechat_open_id, nickname, current_school_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (wechat_open_id) DO UPDATE SET nickname = EXCLUDED.nickname, updated_at = now()
		RETURNING id, nickname, avatar_url, current_school_id
	`, user.ID, "dev:"+externalID, nickname, schoolID).Scan(&user.ID, &user.Nickname, &user.AvatarURL, &user.SchoolID)
	return user, err
}

func (p *Postgres) StoreTokenPair(ctx context.Context, accessHash, refreshHash, userID string, accessExpiresAt, refreshExpiresAt time.Time) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO auth_tokens(token_hash,user_id,kind,expires_at) VALUES($1,$2,'access',$3),($4,$2,'refresh',$5)`, accessHash, userID, accessExpiresAt, refreshHash, refreshExpiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) RotateRefreshToken(ctx context.Context, oldRefreshHash, accessHash, refreshHash string, accessExpiresAt, refreshExpiresAt time.Time) (domain.User, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var user domain.User
	err = tx.QueryRow(ctx, `
		UPDATE auth_tokens t SET revoked_at=now()
		FROM users u
		WHERE t.token_hash=$1 AND t.user_id=u.id AND t.kind='refresh'
		  AND t.revoked_at IS NULL AND t.expires_at>now() AND u.status='active'
		RETURNING u.id,u.nickname,u.avatar_url,u.current_school_id
	`, oldRefreshHash).Scan(&user.ID, &user.Nickname, &user.AvatarURL, &user.SchoolID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ErrNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO auth_tokens(token_hash,user_id,kind,expires_at) VALUES($1,$2,'access',$3),($4,$2,'refresh',$5)`, accessHash, user.ID, accessExpiresAt, refreshHash, refreshExpiresAt); err != nil {
		return domain.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (p *Postgres) UserByToken(ctx context.Context, hash, kind string) (domain.User, error) {
	var user domain.User
	err := p.pool.QueryRow(ctx, `
		SELECT u.id, u.nickname, u.avatar_url, u.current_school_id
		FROM auth_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = $1 AND t.kind = $2 AND t.revoked_at IS NULL
		  AND t.expires_at > now() AND u.status = 'active'
	`, hash, kind).Scan(&user.ID, &user.Nickname, &user.AvatarURL, &user.SchoolID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ErrNotFound
	}
	return user, err
}

func (p *Postgres) CreateSession(ctx context.Context, userID, schoolID, title string) (domain.Session, error) {
	now := time.Now().UTC()
	session := domain.Session{ID: id.New("ses"), Title: title, CreatedAt: now, UpdatedAt: now, Messages: []domain.Message{}}
	_, err := p.pool.Exec(ctx, `INSERT INTO sessions(id,user_id,school_id,title,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$5)`, session.ID, userID, schoolID, title, now)
	return session, err
}

func (p *Postgres) ListSessions(ctx context.Context, userID string) ([]domain.Session, error) {
	rows, err := p.pool.Query(ctx, `SELECT id,title,created_at,updated_at FROM sessions WHERE user_id=$1 ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := make([]domain.Session, 0)
	positions := make(map[string]int)
	for rows.Next() {
		var session domain.Session
		if err := rows.Scan(&session.ID, &session.Title, &session.CreatedAt, &session.UpdatedAt); err != nil {
			return nil, err
		}
		session.Messages = []domain.Message{}
		positions[session.ID] = len(sessions)
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if len(sessions) == 0 {
		return sessions, nil
	}
	messageRows, err := p.pool.Query(ctx, `
		SELECT m.id,m.session_id,m.role,m.content,m.created_at,m.status,
		       COALESCE(array_agg(ms.source_id ORDER BY ms.position) FILTER (WHERE ms.source_id IS NOT NULL), ARRAY[]::text[])
		FROM messages m
		JOIN sessions s ON s.id=m.session_id
		LEFT JOIN message_sources ms ON ms.message_id=m.id
		WHERE s.user_id=$1
		GROUP BY m.id
		ORDER BY m.created_at,m.id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer messageRows.Close()
	for messageRows.Next() {
		var message domain.Message
		if err := messageRows.Scan(&message.ID, &message.SessionID, &message.Role, &message.Content, &message.CreatedAt, &message.Status, &message.SourceIDs); err != nil {
			return nil, err
		}
		if position, ok := positions[message.SessionID]; ok {
			sessions[position].Messages = append(sessions[position].Messages, message)
		}
	}
	return sessions, messageRows.Err()
}

func (p *Postgres) GetSession(ctx context.Context, userID, sessionID string) (domain.Session, error) {
	var session domain.Session
	err := p.pool.QueryRow(ctx, `SELECT id,title,created_at,updated_at FROM sessions WHERE id=$1 AND user_id=$2`, sessionID, userID).Scan(&session.ID, &session.Title, &session.CreatedAt, &session.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, ErrNotFound
	}
	if err != nil {
		return domain.Session{}, err
	}
	session.Messages, err = p.ListMessages(ctx, userID, sessionID)
	return session, err
}

func (p *Postgres) DeleteSession(ctx context.Context, userID, sessionID string) error {
	command, err := p.pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1 AND user_id=$2`, sessionID, userID)
	if err == nil && command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (p *Postgres) ClearSessions(ctx context.Context, userID string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, userID)
	return err
}

func (p *Postgres) CreateMessage(ctx context.Context, userID string, message domain.Message) (domain.Message, error) {
	if message.ID == "" {
		message.ID = id.New("msg")
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	command, err := p.pool.Exec(ctx, `
		INSERT INTO messages(id,session_id,role,content,status,created_at)
		SELECT $1,$2,$3,$4,$5,$6 FROM sessions WHERE id=$2 AND user_id=$7
	`, message.ID, message.SessionID, message.Role, message.Content, message.Status, message.CreatedAt, userID)
	if err == nil && command.RowsAffected() == 0 {
		return domain.Message{}, ErrNotFound
	}
	if err != nil {
		return domain.Message{}, err
	}
	_, err = p.pool.Exec(ctx, `UPDATE sessions SET updated_at=$1 WHERE id=$2`, message.CreatedAt, message.SessionID)
	return message, err
}

// CreateUserMessageAndRun prevents a user message from being persisted without
// its corresponding AgentRun when either insert fails.
func (p *Postgres) CreateUserMessageAndRun(ctx context.Context, userID string, message domain.Message) (domain.Message, domain.AgentRun, error) {
	if message.ID == "" {
		message.ID = id.New("msg")
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.Message{}, domain.AgentRun{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	command, err := tx.Exec(ctx, `
		INSERT INTO messages(id,session_id,role,content,status,created_at)
		SELECT $1,$2,$3,$4,$5,$6 FROM sessions WHERE id=$2 AND user_id=$7
	`, message.ID, message.SessionID, message.Role, message.Content, message.Status, message.CreatedAt, userID)
	if err != nil {
		return domain.Message{}, domain.AgentRun{}, err
	}
	if command.RowsAffected() == 0 {
		return domain.Message{}, domain.AgentRun{}, ErrNotFound
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET updated_at=$1 WHERE id=$2`, message.CreatedAt, message.SessionID); err != nil {
		return domain.Message{}, domain.AgentRun{}, err
	}
	run := domain.AgentRun{ID: id.New("run"), SessionID: message.SessionID, Status: "created", CreatedAt: time.Now().UTC()}
	if _, err := tx.Exec(ctx, `INSERT INTO agent_runs(id,session_id,status,created_at,updated_at) VALUES($1,$2,'QUEUED',$3,$3)`, run.ID, run.SessionID, run.CreatedAt); err != nil {
		return domain.Message{}, domain.AgentRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Message{}, domain.AgentRun{}, err
	}
	return message, run, nil
}

// CompleteAssistantMessage atomically persists the final answer and all source
// links so history never exposes a partially attached answer.
func (p *Postgres) CompleteAssistantMessage(ctx context.Context, userID string, message domain.Message, sources []domain.Source) (domain.Message, error) {
	if message.ID == "" {
		message.ID = id.New("msg")
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.Message{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	command, err := tx.Exec(ctx, `
		INSERT INTO messages(id,session_id,role,content,status,created_at)
		SELECT $1,$2,$3,$4,$5,$6 FROM sessions WHERE id=$2 AND user_id=$7
	`, message.ID, message.SessionID, message.Role, message.Content, message.Status, message.CreatedAt, userID)
	if err != nil {
		return domain.Message{}, err
	}
	if command.RowsAffected() == 0 {
		return domain.Message{}, ErrNotFound
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET updated_at=$1 WHERE id=$2`, message.CreatedAt, message.SessionID); err != nil {
		return domain.Message{}, err
	}
	for position, source := range sources {
		if _, err := tx.Exec(ctx, `INSERT INTO message_sources(message_id,source_id,position) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, message.ID, source.ID, position); err != nil {
			return domain.Message{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Message{}, err
	}
	return message, nil
}

func (p *Postgres) ListMessages(ctx context.Context, userID, sessionID string) ([]domain.Message, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT m.id,m.session_id,m.role,m.content,m.created_at,m.status,
		       COALESCE(array_agg(ms.source_id ORDER BY ms.position) FILTER (WHERE ms.source_id IS NOT NULL), ARRAY[]::text[])
		FROM messages m
		JOIN sessions s ON s.id=m.session_id
		LEFT JOIN message_sources ms ON ms.message_id=m.id
		WHERE m.session_id=$1 AND s.user_id=$2
		GROUP BY m.id
		ORDER BY m.created_at,m.id
	`, sessionID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]domain.Message, 0)
	for rows.Next() {
		var message domain.Message
		if err := rows.Scan(&message.ID, &message.SessionID, &message.Role, &message.Content, &message.CreatedAt, &message.Status, &message.SourceIDs); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (p *Postgres) CreateRun(ctx context.Context, sessionID string) (domain.AgentRun, error) {
	run := domain.AgentRun{ID: id.New("run"), SessionID: sessionID, Status: "created", CreatedAt: time.Now().UTC()}
	_, err := p.pool.Exec(ctx, `INSERT INTO agent_runs(id,session_id,status,created_at,updated_at) VALUES($1,$2,'QUEUED',$3,$3)`, run.ID, run.SessionID, run.CreatedAt)
	return run, err
}

func (p *Postgres) UpdateRunStatus(ctx context.Context, runID, status, errorCode string) error {
	_, err := p.pool.Exec(ctx, `UPDATE agent_runs SET status=$2,error_code=$3,updated_at=now() WHERE id=$1`, runID, status, errorCode)
	return err
}

// FinalizeRun changes terminal state and appends its public terminal event in
// one transaction. The row lock makes competing completion/cancellation calls
// idempotent and guarantees at most one terminal event.
func (p *Postgres) FinalizeRun(ctx context.Context, runID, status, errorCode, eventType string, payload any) (domain.RunEvent, bool, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return domain.RunEvent{}, false, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.RunEvent{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM agent_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&currentStatus); errors.Is(err, pgx.ErrNoRows) {
		return domain.RunEvent{}, false, ErrNotFound
	} else if err != nil {
		return domain.RunEvent{}, false, err
	}
	if currentStatus == "COMPLETED" || currentStatus == "FAILED" || currentStatus == "CANCELLED" {
		return domain.RunEvent{}, false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE agent_runs SET status=$2,error_code=$3,updated_at=now() WHERE id=$1`, runID, status, errorCode); err != nil {
		return domain.RunEvent{}, false, err
	}
	event := domain.RunEvent{RunID: runID, Type: eventType, Payload: encoded}
	if err := tx.QueryRow(ctx, `INSERT INTO run_events(run_id,event_type,payload) VALUES($1,$2,$3) RETURNING sequence,created_at`, runID, eventType, encoded).Scan(&event.Sequence, &event.CreatedAt); err != nil {
		return domain.RunEvent{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.RunEvent{}, false, err
	}
	return event, true, nil
}

func (p *Postgres) RunOwner(ctx context.Context, runID string) (string, string, string, error) {
	var userID, sessionID, status string
	err := p.pool.QueryRow(ctx, `SELECT s.user_id,r.session_id,r.status FROM agent_runs r JOIN sessions s ON s.id=r.session_id WHERE r.id=$1`, runID).Scan(&userID, &sessionID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrNotFound
	}
	return userID, sessionID, status, err
}

func (p *Postgres) AppendRunEvent(ctx context.Context, runID, eventType string, payload any) (domain.RunEvent, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return domain.RunEvent{}, err
	}
	event := domain.RunEvent{RunID: runID, Type: eventType, Payload: encoded}
	err = p.pool.QueryRow(ctx, `INSERT INTO run_events(run_id,event_type,payload) VALUES($1,$2,$3) RETURNING sequence,created_at`, runID, eventType, encoded).Scan(&event.Sequence, &event.CreatedAt)
	return event, err
}

func (p *Postgres) ListRunEvents(ctx context.Context, runID string, after int64) ([]domain.RunEvent, error) {
	rows, err := p.pool.Query(ctx, `SELECT sequence,run_id,event_type,payload,created_at FROM run_events WHERE run_id=$1 AND sequence>$2 ORDER BY sequence`, runID, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]domain.RunEvent, 0)
	for rows.Next() {
		var event domain.RunEvent
		if err := rows.Scan(&event.Sequence, &event.RunID, &event.Type, &event.Payload, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (p *Postgres) UpsertSource(ctx context.Context, schoolID string, source domain.Source) error {
	command, err := p.pool.Exec(ctx, `
		INSERT INTO sources(id,school_id,title,publisher,published_at,audience,summary,url,official)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT(id) DO UPDATE SET title=EXCLUDED.title,publisher=EXCLUDED.publisher,published_at=EXCLUDED.published_at,audience=EXCLUDED.audience,summary=EXCLUDED.summary,url=EXCLUDED.url,official=EXCLUDED.official,updated_at=now()
		WHERE sources.school_id=EXCLUDED.school_id
	`, source.ID, schoolID, source.Title, source.Publisher, source.PublishedAt, source.Audience, source.Summary, source.URL, source.Official)
	if err == nil && command.RowsAffected() == 0 {
		return fmt.Errorf("source %q belongs to a different school", source.ID)
	}
	return err
}

func (p *Postgres) AttachSources(ctx context.Context, messageID string, sources []domain.Source) error {
	for position, source := range sources {
		if _, err := p.pool.Exec(ctx, `INSERT INTO message_sources(message_id,source_id,position) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, messageID, source.ID, position); err != nil {
			return err
		}
	}
	return nil
}

func (p *Postgres) GetSource(ctx context.Context, sourceID string) (domain.Source, error) {
	var source domain.Source
	err := p.pool.QueryRow(ctx, `SELECT id,title,publisher,published_at,audience,summary,url,official FROM sources WHERE id=$1`, sourceID).Scan(&source.ID, &source.Title, &source.Publisher, &source.PublishedAt, &source.Audience, &source.Summary, &source.URL, &source.Official)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Source{}, ErrNotFound
	}
	return source, err
}

func (p *Postgres) GetSourceForUser(ctx context.Context, userID, sourceID string) (domain.Source, error) {
	var source domain.Source
	err := p.pool.QueryRow(ctx, `
		SELECT s.id,s.title,s.publisher,s.published_at,s.audience,s.summary,s.url,s.official
		FROM sources s JOIN users u ON u.current_school_id=s.school_id
		WHERE s.id=$1 AND u.id=$2 AND u.status='active'
	`, sourceID, userID).Scan(&source.ID, &source.Title, &source.Publisher, &source.PublishedAt, &source.Audience, &source.Summary, &source.URL, &source.Official)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Source{}, ErrNotFound
	}
	return source, err
}

func (p *Postgres) CreateFeedback(ctx context.Context, userID, messageID, value string) (domain.Feedback, error) {
	feedback := domain.Feedback{ID: id.New("fb"), MessageID: messageID, Value: value, CreatedAt: time.Now().UTC()}
	command, err := p.pool.Exec(ctx, `
		INSERT INTO feedback(id,user_id,message_id,value,created_at)
		SELECT $1,$2,$3,$4,$5 FROM messages m JOIN sessions s ON s.id=m.session_id WHERE m.id=$3 AND s.user_id=$2
	`, feedback.ID, userID, messageID, value, feedback.CreatedAt)
	if err == nil && command.RowsAffected() == 0 {
		return domain.Feedback{}, ErrNotFound
	}
	return feedback, err
}

func (p *Postgres) RecordUsage(ctx context.Context, record domain.UsageRecord) error {
	if record.ID == "" {
		record.ID = id.New("use")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	var runID any
	if record.RunID != "" {
		runID = record.RunID
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO usage_records(
			id,user_id,run_id,provider,model,input_tokens,output_tokens,
			estimated_cost_micro_rmb,latency_ms,status,error_code,tokens_estimated,created_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`, record.ID, record.UserID, runID, record.Provider, record.Model,
		record.InputTokens, record.OutputTokens, record.EstimatedCostMicroRMB,
		record.LatencyMS, record.Status, record.ErrorCode, record.TokensEstimated, record.CreatedAt)
	return err
}
