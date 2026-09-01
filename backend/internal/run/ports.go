package run

import (
	"context"

	"asku/backend/internal/domain"
)

// Repository is defined by the lifecycle consumer. PostgreSQL is one adapter;
// tests and future durable stores can satisfy the same boundary.
type Repository interface {
	CreateUserMessageAndRun(ctx context.Context, userID string, message domain.Message) (domain.Message, domain.AgentRun, error)
	CompleteAssistantMessage(ctx context.Context, userID string, message domain.Message, sources []domain.Source) (domain.Message, error)
	RunOwner(ctx context.Context, runID string) (userID, sessionID, status string, err error)
	UpdateRunStatus(ctx context.Context, runID, status, errorCode string) error
	FinalizeRun(ctx context.Context, runID, status, errorCode, eventType string, payload any) (domain.RunEvent, bool, error)
	UpsertSource(ctx context.Context, schoolID string, source domain.Source) error
	AppendRunEvent(ctx context.Context, runID, eventType string, payload any) (domain.RunEvent, error)
}
