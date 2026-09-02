package store

import (
	"context"
	"fmt"
	"time"

	"asku/backend/internal/observability"
	"github.com/jackc/pgx/v5"
)

// Overview builds a consistent, read-only operational snapshot. AgentRun
// events remain the source of truth for routes, cache hits and latency.
func (p *Postgres) Overview(ctx context.Context, window observability.Window) (observability.Overview, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return observability.Overview{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	report := observability.Overview{}
	windowArgs := []any{window.From, window.To, window.SchoolID}
	zonedWindowArgs := append(windowArgs, window.TimeZone)
	if err := scanUserMetrics(ctx, tx, zonedWindowArgs, &report); err != nil {
		return observability.Overview{}, fmt.Errorf("query user metrics: %w", err)
	}
	if err := scanEngagement(ctx, tx, windowArgs, &report); err != nil {
		return observability.Overview{}, fmt.Errorf("query engagement metrics: %w", err)
	}
	if err := scanRunMetrics(ctx, tx, windowArgs, &report); err != nil {
		return observability.Overview{}, fmt.Errorf("query run metrics: %w", err)
	}
	if err := scanCostMetrics(ctx, tx, windowArgs, &report); err != nil {
		return observability.Overview{}, fmt.Errorf("query cost metrics: %w", err)
	}
	if report.Routes, err = scanCounts(ctx, tx, routeBreakdownSQL, windowArgs); err != nil {
		return observability.Overview{}, fmt.Errorf("query route breakdown: %w", err)
	}
	if report.ErrorCodes, err = scanCounts(ctx, tx, errorBreakdownSQL, windowArgs); err != nil {
		return observability.Overview{}, fmt.Errorf("query error breakdown: %w", err)
	}
	if report.TopQuestions, err = scanTopQuestions(ctx, tx, windowArgs); err != nil {
		return observability.Overview{}, fmt.Errorf("query top questions: %w", err)
	}
	if report.Daily, err = scanDaily(ctx, tx, zonedWindowArgs); err != nil {
		return observability.Overview{}, fmt.Errorf("query daily metrics: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return observability.Overview{}, err
	}
	report.Finalize(window, time.Now())
	return report, nil
}

func scanUserMetrics(ctx context.Context, tx pgx.Tx, args []any, report *observability.Overview) error {
	return tx.QueryRow(ctx, `
		WITH cohort AS (
			SELECT u.id, (u.created_at AT TIME ZONE $4)::date AS cohort_day,
			       (u.created_at < $2::timestamptz - interval '1 day') AS d1_eligible,
			       (u.created_at < $2::timestamptz - interval '7 days') AS d7_eligible
			FROM users u
			WHERE u.current_school_id=$3 AND u.created_at >= $1 AND u.created_at < $2
		), retained AS (
			SELECT c.*,
			       EXISTS (
				SELECT 1 FROM messages m JOIN sessions s ON s.id=m.session_id
				WHERE s.user_id=c.id AND m.role='user'
				  AND (m.created_at AT TIME ZONE $4)::date=c.cohort_day + 1
			) AS d1_retained,
			       EXISTS (
				SELECT 1 FROM messages m JOIN sessions s ON s.id=m.session_id
				WHERE s.user_id=c.id AND m.role='user'
				  AND (m.created_at AT TIME ZONE $4)::date=c.cohort_day + 7
			) AS d7_retained
			FROM cohort c
		)
		SELECT
			(SELECT count(*) FROM users WHERE current_school_id=$3 AND created_at < $2),
			(SELECT count(*) FROM cohort),
			(SELECT count(DISTINCT s.user_id) FROM messages m JOIN sessions s ON s.id=m.session_id
			 WHERE s.school_id=$3 AND m.role='user' AND m.created_at >= $1 AND m.created_at < $2),
			count(*) FILTER (WHERE d1_eligible), count(*) FILTER (WHERE d1_eligible AND d1_retained),
			count(*) FILTER (WHERE d7_eligible), count(*) FILTER (WHERE d7_eligible AND d7_retained)
		FROM retained
	`, args...).Scan(
		&report.Users.TotalRegistered, &report.Users.NewRegistered, &report.Users.ActiveUsers,
		&report.Users.D1Eligible, &report.Users.D1Retained, &report.Users.D7Eligible, &report.Users.D7Retained,
	)
}

func scanEngagement(ctx context.Context, tx pgx.Tx, args []any, report *observability.Overview) error {
	return tx.QueryRow(ctx, `
		SELECT count(*), count(DISTINCT m.session_id)
		FROM messages m JOIN sessions s ON s.id=m.session_id
		WHERE s.school_id=$3 AND m.role='user' AND m.created_at >= $1 AND m.created_at < $2
	`, args...).Scan(&report.Engagement.Questions, &report.Engagement.ActiveSessions)
}

func scanRunMetrics(ctx context.Context, tx pgx.Tx, args []any, report *observability.Overview) error {
	return tx.QueryRow(ctx, runFactsCTE+`
		SELECT count(*),
		       count(*) FILTER (WHERE status='COMPLETED'),
		       count(*) FILTER (WHERE status='FAILED'),
		       count(*) FILTER (WHERE status='CANCELLED'),
		       count(*) FILTER (
			 WHERE status='COMPLETED' AND route <> 'controlled' AND assistant_message_id <> ''
			   AND NOT EXISTS (SELECT 1 FROM message_citations mc WHERE mc.message_id=assistant_message_id)
		       ),
		       COALESCE(avg(latency_ms) FILTER (WHERE latency_ms IS NOT NULL),0)::double precision,
		       COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE latency_ms IS NOT NULL),0)::double precision,
		       COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE latency_ms IS NOT NULL),0)::double precision,
		       COALESCE(avg(ttft_ms) FILTER (WHERE ttft_ms IS NOT NULL),0)::double precision,
		       COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY ttft_ms) FILTER (WHERE ttft_ms IS NOT NULL),0)::double precision,
		       (SELECT count(*) FROM feedback f JOIN messages m ON m.id=f.message_id JOIN sessions s ON s.id=m.session_id
			WHERE s.school_id=$3 AND f.value='unhelpful' AND f.created_at >= $1 AND f.created_at < $2)
		FROM run_facts
	`, args...).Scan(
		&report.Quality.Runs, &report.Quality.CompletedRuns, &report.Quality.FailedRuns,
		&report.Quality.CancelledRuns, &report.Quality.NoSourceAnswers,
		&report.Performance.AverageLatencyMS, &report.Performance.P50LatencyMS, &report.Performance.P95LatencyMS,
		&report.Performance.AverageTTFTMS, &report.Performance.P95TTFTMS, &report.Quality.Unhelpful,
	)
}

func scanCostMetrics(ctx context.Context, tx pgx.Tx, args []any, report *observability.Overview) error {
	return tx.QueryRow(ctx, runFactsCTE+`, usage AS (
		SELECT COALESCE(sum(u.input_tokens),0) input_tokens,
		       COALESCE(sum(u.output_tokens),0) output_tokens,
		       COALESCE(sum(u.estimated_cost_micro_rmb),0) cost
		FROM usage_records u
		JOIN agent_runs r ON r.id=u.run_id JOIN sessions s ON s.id=r.session_id
		WHERE s.school_id=$3 AND u.created_at >= $1 AND u.created_at < $2
	)
	SELECT usage.input_tokens, usage.output_tokens, usage.cost,
	       (SELECT count(*) FROM run_facts WHERE route='cache')
	FROM usage
	`, args...).Scan(&report.Cost.InputTokens, &report.Cost.OutputTokens, &report.Cost.EstimatedCostMicroRMB, &report.Cost.CacheHits)
}

const runFactsCTE = `
	WITH scoped_runs AS (
		SELECT r.id, r.status, r.error_code, r.created_at
		FROM agent_runs r JOIN sessions s ON s.id=r.session_id
		WHERE s.school_id=$3 AND r.created_at >= $1 AND r.created_at < $2
	), event_facts AS (
		SELECT e.run_id,
		       COALESCE(max(e.payload->>'route') FILTER (WHERE e.event_type='route.resolved'),'') AS route,
		       min(e.created_at) FILTER (WHERE e.event_type='message.delta') AS first_delta_at,
		       min(e.created_at) FILTER (WHERE e.event_type IN ('run.completed','run.failed')) AS terminal_at,
		       COALESCE(max(e.payload->'message'->>'id') FILTER (WHERE e.event_type='message.completed'),'') AS assistant_message_id
		FROM run_events e JOIN scoped_runs r ON r.id=e.run_id
		GROUP BY e.run_id
	), run_facts AS (
		SELECT r.id, r.status, r.error_code, COALESCE(ef.route,'') route,
		       COALESCE(ef.assistant_message_id,'') assistant_message_id,
		       CASE WHEN ef.terminal_at IS NULL THEN NULL ELSE extract(epoch FROM (ef.terminal_at-r.created_at))*1000 END AS latency_ms,
		       CASE WHEN ef.first_delta_at IS NULL THEN NULL ELSE extract(epoch FROM (ef.first_delta_at-r.created_at))*1000 END AS ttft_ms
		FROM scoped_runs r LEFT JOIN event_facts ef ON ef.run_id=r.id
	)`

const routeBreakdownSQL = runFactsCTE + `
	SELECT CASE WHEN route='' THEN 'unresolved' ELSE route END, count(*)
	FROM run_facts GROUP BY 1 ORDER BY count(*) DESC, 1 LIMIT 20`

const errorBreakdownSQL = runFactsCTE + `
	SELECT error_code, count(*) FROM run_facts
	WHERE error_code <> '' GROUP BY error_code ORDER BY count(*) DESC, error_code LIMIT 20`

func scanCounts(ctx context.Context, tx pgx.Tx, sql string, args []any) ([]observability.CountMetric, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]observability.CountMetric, 0)
	for rows.Next() {
		var item observability.CountMetric
		if err := rows.Scan(&item.Name, &item.Count); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func scanTopQuestions(ctx context.Context, tx pgx.Tx, args []any) ([]observability.QuestionMetric, error) {
	rows, err := tx.Query(ctx, `
		SELECT left(regexp_replace(trim(m.content), '[[:space:]]+', ' ', 'g'), 200) AS question, count(*)
		FROM messages m JOIN sessions s ON s.id=m.session_id
		WHERE s.school_id=$3 AND m.role='user' AND m.created_at >= $1 AND m.created_at < $2
		GROUP BY 1 ORDER BY count(*) DESC, question LIMIT 10
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]observability.QuestionMetric, 0)
	for rows.Next() {
		var item observability.QuestionMetric
		if err := rows.Scan(&item.Question, &item.Count); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func scanDaily(ctx context.Context, tx pgx.Tx, args []any) ([]observability.DailyMetric, error) {
	rows, err := tx.Query(ctx, `
		WITH days AS (
			SELECT generate_series(
				($1::timestamptz AT TIME ZONE $4)::date,
				(($2::timestamptz - interval '1 microsecond') AT TIME ZONE $4)::date,
				interval '1 day'
			)::date AS day
		), user_daily AS (
			SELECT (created_at AT TIME ZONE $4)::date AS day, count(*) AS count
			FROM users WHERE current_school_id=$3 AND created_at >= $1 AND created_at < $2 GROUP BY 1
		), question_daily AS (
			SELECT (m.created_at AT TIME ZONE $4)::date AS day, count(*) AS questions, count(DISTINCT s.user_id) AS active_users
			FROM messages m JOIN sessions s ON s.id=m.session_id
			WHERE s.school_id=$3 AND m.role='user' AND m.created_at >= $1 AND m.created_at < $2 GROUP BY 1
		), run_daily AS (
			SELECT (r.created_at AT TIME ZONE $4)::date AS day, count(*) AS runs,
			       count(*) FILTER (WHERE r.status='COMPLETED') completed,
			       count(*) FILTER (WHERE r.status='FAILED') failed
			FROM agent_runs r JOIN sessions s ON s.id=r.session_id
			WHERE s.school_id=$3 AND r.created_at >= $1 AND r.created_at < $2 GROUP BY 1
		), cost_daily AS (
			SELECT (u.created_at AT TIME ZONE $4)::date AS day, COALESCE(sum(u.estimated_cost_micro_rmb),0) AS cost
			FROM usage_records u JOIN agent_runs r ON r.id=u.run_id JOIN sessions s ON s.id=r.session_id
			WHERE s.school_id=$3 AND u.created_at >= $1 AND u.created_at < $2 GROUP BY 1
		)
		SELECT to_char(d.day,'YYYY-MM-DD'), COALESCE(ud.count,0), COALESCE(qd.active_users,0),
		       COALESCE(qd.questions,0), COALESCE(rd.runs,0), COALESCE(rd.completed,0),
		       COALESCE(rd.failed,0), COALESCE(cd.cost,0)
		FROM days d LEFT JOIN user_daily ud USING(day) LEFT JOIN question_daily qd USING(day)
		LEFT JOIN run_daily rd USING(day) LEFT JOIN cost_daily cd USING(day) ORDER BY d.day
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]observability.DailyMetric, 0)
	for rows.Next() {
		var item observability.DailyMetric
		if err := rows.Scan(&item.Date, &item.NewUsers, &item.ActiveUsers, &item.Questions, &item.Runs,
			&item.CompletedRuns, &item.FailedRuns, &item.EstimatedCostMicroRMB); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
