package run

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"asku/backend/internal/agent"
	"asku/backend/internal/domain"
	"asku/backend/internal/id"
)

type Service struct {
	store    Repository
	executor agent.Executor
	hub      *Hub
	delays   bool
}

func NewService(repository Repository, executor agent.Executor, hub *Hub, delays bool) *Service {
	return &Service{store: repository, executor: executor, hub: hub, delays: delays}
}

func (s *Service) Start(ctx context.Context, userID, schoolID, sessionID, question, userMessageID string) (domain.AgentRun, domain.Message, error) {
	userMessage := domain.Message{
		ID: userMessageID, SessionID: sessionID, Role: "user", Content: question,
		CreatedAt: time.Now().UTC(), Status: "completed",
	}
	createdMessage, run, err := s.store.CreateUserMessageAndRun(ctx, userID, userMessage)
	if err != nil {
		return domain.AgentRun{}, domain.Message{}, err
	}
	workerContext, cancel := context.WithCancel(context.Background())
	s.hub.RegisterCancel(run.ID, cancel)
	go func() {
		defer cancel()
		s.execute(workerContext, userID, schoolID, run, question)
	}()
	return run, createdMessage, nil
}

func (s *Service) Cancel(ctx context.Context, userID, runID string) error {
	ownerID, _, status, err := s.store.RunOwner(ctx, runID)
	if err != nil {
		return err
	}
	if ownerID != userID {
		return domain.ErrNotFound
	}
	if isTerminalStatus(status) {
		return nil
	}
	if !s.hub.Cancel(runID) {
		return s.finalize(ctx, runID, "CANCELLED", "cancel_requested", "run.failed", map[string]any{
			"error": "已停止生成。", "retryable": false, "code": "cancelled",
		})
	}
	return nil
}

func (s *Service) execute(ctx context.Context, userID, schoolID string, run domain.AgentRun, question string) {
	defer s.hub.UnregisterCancel(run.ID)
	if err := s.emit(ctx, run.ID, "run.started", map[string]any{
		"run":       domain.AgentRun{ID: run.ID, SessionID: run.SessionID, Status: "created", CreatedAt: run.CreatedAt},
		"sessionId": run.SessionID,
	}); err != nil {
		s.fail(run.ID, err)
		return
	}
	messageID := id.New("msg")
	progress := &executionProgress{service: s, runID: run.ID, schoolID: schoolID, messageID: messageID}
	result, err := s.executor.Execute(ctx, agent.ExecutionRequest{UserID: userID, SchoolID: schoolID, RunID: run.ID, Question: question}, progress)
	if err != nil {
		var executionErr *agent.ExecutionError
		if errors.As(err, &executionErr) {
			slog.Error("agent execution failed", "run_id", run.ID, "code", executionErr.Code, "error", executionErr.Cause)
			s.failWithMessage(run.ID, executionErr.Code, executionErr.Message, executionErr.Retryable)
			return
		}
		s.fail(run.ID, err)
		return
	}

	message := domain.Message{
		ID: messageID, SessionID: run.SessionID, Role: "assistant", Content: result.Answer,
		CreatedAt: time.Now().UTC(), Status: "completed",
	}
	for _, source := range result.Sources {
		message.SourceIDs = append(message.SourceIDs, source.ID)
	}
	message, err = s.store.CompleteAssistantMessage(ctx, userID, message, result.Sources)
	if err != nil {
		s.fail(run.ID, err)
		return
	}
	if err := s.emit(ctx, run.ID, "message.completed", map[string]any{"message": message}); err != nil {
		s.fail(run.ID, err)
		return
	}
	if err := s.finalize(ctx, run.ID, "COMPLETED", "", "run.completed", map[string]any{"runId": run.ID}); err != nil {
		slog.Error("finalize completed run", "run_id", run.ID, "error", err)
	}
}

type executionProgress struct {
	service   *Service
	runID     string
	schoolID  string
	messageID string
}

func (p *executionProgress) RouteResolved(ctx context.Context, route, reason string) error {
	if !p.service.pause(ctx, 180*time.Millisecond) {
		return ctx.Err()
	}
	if err := p.service.store.UpdateRunStatus(ctx, p.runID, "ROUTING", ""); err != nil {
		return err
	}
	return p.service.emit(ctx, p.runID, "route.resolved", map[string]any{"route": route, "reason": reason})
}

func (p *executionProgress) RetrievalStarted(ctx context.Context, engine string) error {
	if !p.service.pause(ctx, 220*time.Millisecond) {
		return ctx.Err()
	}
	if err := p.service.store.UpdateRunStatus(ctx, p.runID, "RETRIEVING", ""); err != nil {
		return err
	}
	return p.service.emit(ctx, p.runID, "retrieval.started", map[string]any{"engine": engine})
}

func (p *executionProgress) RetrievalCompleted(ctx context.Context, engine string, hits int, metadata map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload := map[string]any{"engine": engine, "hits": hits}
	for key, value := range metadata {
		payload[key] = value
	}
	return p.service.emit(ctx, p.runID, "retrieval.completed", payload)
}

func (p *executionProgress) SourcesUpdated(ctx context.Context, sources []domain.Source, metadata map[string]any) error {
	if !p.service.pause(ctx, 360*time.Millisecond) {
		return ctx.Err()
	}
	for _, source := range sources {
		if err := p.service.store.UpsertSource(ctx, p.schoolID, source); err != nil {
			return err
		}
	}
	payload := map[string]any{"sources": sources}
	for key, value := range metadata {
		payload[key] = value
	}
	return p.service.emit(ctx, p.runID, "sources.updated", payload)
}

func (p *executionProgress) GenerationStarted(ctx context.Context, provider string) error {
	if !p.service.pause(ctx, 260*time.Millisecond) {
		return ctx.Err()
	}
	if err := p.service.store.UpdateRunStatus(ctx, p.runID, "GENERATING", ""); err != nil {
		return err
	}
	return p.service.emit(ctx, p.runID, "generation.started", map[string]any{"provider": provider})
}

func (p *executionProgress) MessageDelta(ctx context.Context, delta string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.service.emit(ctx, p.runID, "message.delta", map[string]any{"messageId": p.messageID, "delta": delta})
}

func (s *Service) emit(ctx context.Context, runID, eventType string, payload any) error {
	event, err := s.store.AppendRunEvent(ctx, runID, eventType, payload)
	if err != nil {
		return err
	}
	s.hub.Publish(event)
	return nil
}

func (s *Service) pause(ctx context.Context, duration time.Duration) bool {
	if !s.delays {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *Service) fail(runID string, err error) {
	if errors.Is(err, context.Canceled) {
		s.cancelled(runID)
		return
	}
	slog.Error("agent run failed", "run_id", runID, "error", err)
	s.failWithMessage(runID, "internal_error", "生成失败，请稍后重试。", true)
}

func (s *Service) failWithMessage(runID, code, message string, retryable bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.finalize(ctx, runID, "FAILED", code, "run.failed", map[string]any{"error": message, "retryable": retryable, "code": code}); err != nil {
		slog.Error("finalize failed run", "run_id", runID, "error", err)
	}
}

func (s *Service) cancelled(runID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.finalize(ctx, runID, "CANCELLED", "cancelled", "run.failed", map[string]any{"error": "已停止生成。", "retryable": false, "code": "cancelled"}); err != nil {
		slog.Error("finalize cancelled run", "run_id", runID, "error", err)
	}
}

func (s *Service) finalize(ctx context.Context, runID, status, errorCode, eventType string, payload any) error {
	event, changed, err := s.store.FinalizeRun(ctx, runID, status, errorCode, eventType, payload)
	if err != nil {
		return err
	}
	if changed {
		s.hub.Publish(event)
	}
	return nil
}

func isTerminalStatus(status string) bool {
	return status == "COMPLETED" || status == "FAILED" || status == "CANCELLED"
}
