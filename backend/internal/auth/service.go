package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"asku/backend/internal/domain"
	"asku/backend/internal/httpx"
)

type userContextKey struct{}

type Service struct {
	store      Repository
	schools    SchoolRegistry
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewService(database Repository, schools SchoolRegistry, accessTTL, refreshTTL time.Duration) *Service {
	return &Service{store: database, schools: schools, accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (s *Service) DevLogin(ctx context.Context, externalID, nickname string) (domain.TokenPair, error) {
	current := s.schools.Current()
	user, err := s.store.UpsertDevUser(ctx, externalID, nickname, current.ID)
	if err != nil {
		return domain.TokenPair{}, err
	}
	user.SchoolName = current.Name
	pair, accessHash, refreshHash, err := s.newPair(user)
	if err != nil {
		return domain.TokenPair{}, err
	}
	if err := s.store.StoreTokenPair(ctx, accessHash, refreshHash, user.ID, pair.AccessExpiresAt, pair.RefreshExpiresAt); err != nil {
		return domain.TokenPair{}, err
	}
	return pair, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (domain.TokenPair, error) {
	pair, accessHash, refreshHash, err := s.newPair(domain.User{})
	if err != nil {
		return domain.TokenPair{}, err
	}
	user, err := s.store.RotateRefreshToken(ctx, tokenHash(refreshToken), accessHash, refreshHash, pair.AccessExpiresAt, pair.RefreshExpiresAt)
	if err != nil {
		return domain.TokenPair{}, err
	}
	if current, ok := s.schools.Get(user.SchoolID); ok {
		user.SchoolName = current.Name
	}
	pair.User = user
	return pair, nil
}

func (s *Service) UserForAccessToken(ctx context.Context, rawToken string) (domain.User, error) {
	user, err := s.store.UserByToken(ctx, tokenHash(rawToken), "access")
	if err != nil {
		return domain.User{}, err
	}
	if current, ok := s.schools.Get(user.SchoolID); ok {
		user.SchoolName = current.Name
	}
	return user, nil
}

func (s *Service) newPair(user domain.User) (domain.TokenPair, string, string, error) {
	access, err := randomToken()
	if err != nil {
		return domain.TokenPair{}, "", "", err
	}
	refresh, err := randomToken()
	if err != nil {
		return domain.TokenPair{}, "", "", err
	}
	now := time.Now().UTC()
	pair := domain.TokenPair{
		AccessToken:      access,
		AccessExpiresAt:  now.Add(s.accessTTL),
		RefreshToken:     refresh,
		RefreshExpiresAt: now.Add(s.refreshTTL),
		User:             user,
	}
	return pair, tokenHash(access), tokenHash(refresh), nil
}

func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(header, "Bearer ") {
			httpx.Error(w, r, &httpx.HandlerError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "请先登录。"})
			return
		}
		user, err := s.UserForAccessToken(r.Context(), strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				httpx.Error(w, r, &httpx.HandlerError{Status: http.StatusUnauthorized, Code: "token_expired", Message: "登录状态已过期，请重新登录。"})
				return
			}
			httpx.Error(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	})
}

func UserFromContext(ctx context.Context) (domain.User, bool) {
	user, ok := ctx.Value(userContextKey{}).(domain.User)
	return user, ok
}

func randomToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
