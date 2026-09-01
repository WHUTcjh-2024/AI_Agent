package auth

import (
	"context"
	"time"

	"asku/backend/internal/domain"
	"asku/backend/internal/school"
)

type Repository interface {
	UpsertDevUser(ctx context.Context, externalID, nickname, schoolID string) (domain.User, error)
	StoreTokenPair(ctx context.Context, accessHash, refreshHash, userID string, accessExpiresAt, refreshExpiresAt time.Time) error
	RotateRefreshToken(ctx context.Context, oldRefreshHash, accessHash, refreshHash string, accessExpiresAt, refreshExpiresAt time.Time) (domain.User, error)
	UserByToken(ctx context.Context, hash, kind string) (domain.User, error)
}

type SchoolRegistry interface {
	Current() school.Context
	Get(id string) (school.Context, bool)
}
