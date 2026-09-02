package api

import (
	"context"
	"net/http"

	"asku/backend/internal/domain"
	"asku/backend/internal/school"
)

type Repository interface {
	Ping(ctx context.Context) error
	CreateSession(ctx context.Context, userID, schoolID, title string) (domain.Session, error)
	ListSessions(ctx context.Context, userID string) ([]domain.Session, error)
	GetSession(ctx context.Context, userID, sessionID string) (domain.Session, error)
	DeleteSession(ctx context.Context, userID, sessionID string) error
	ClearSessions(ctx context.Context, userID string) error
	CreateMessage(ctx context.Context, userID string, message domain.Message) (domain.Message, error)
	ListMessages(ctx context.Context, userID, sessionID string) ([]domain.Message, error)
	RunOwner(ctx context.Context, runID string) (userID, sessionID, status string, err error)
	ListRunEvents(ctx context.Context, runID string, after int64) ([]domain.RunEvent, error)
	GetSourceForUser(ctx context.Context, userID, sourceID string) (domain.Source, error)
	CreateFeedback(ctx context.Context, userID, messageID, value string) (domain.Feedback, error)
}

type Cache interface {
	Ping(ctx context.Context) error
	AllowQuestion(ctx context.Context, userID string, limit int64) (bool, error)
	ReserveIdempotency(ctx context.Context, userID, key string) (bool, error)
	ReleaseIdempotency(ctx context.Context, userID, key string) error
}

type Authenticator interface {
	DevLogin(ctx context.Context, externalID, nickname string) (domain.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (domain.TokenPair, error)
	Middleware(next http.Handler) http.Handler
}

type RunController interface {
	Start(ctx context.Context, userID, schoolID, sessionID, question, userMessageID string) (domain.AgentRun, domain.Message, error)
	Cancel(ctx context.Context, userID, runID string) error
}

type EventHub interface {
	Subscribe(runID string) (<-chan domain.RunEvent, func())
}

type SchoolRegistry interface {
	Current() school.Context
}
