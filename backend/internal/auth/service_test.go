package auth

import (
	"context"
	"testing"
	"time"

	"asku/backend/internal/domain"
	"asku/backend/internal/school"
)

type repositorySpy struct {
	storedPairs int
	rotations   int
	user        domain.User
}

func (r *repositorySpy) UpsertDevUser(context.Context, string, string, string) (domain.User, error) {
	return r.user, nil
}
func (r *repositorySpy) StoreTokenPair(context.Context, string, string, string, time.Time, time.Time) error {
	r.storedPairs++
	return nil
}
func (r *repositorySpy) RotateRefreshToken(context.Context, string, string, string, time.Time, time.Time) (domain.User, error) {
	r.rotations++
	return r.user, nil
}
func (r *repositorySpy) UserByToken(context.Context, string, string) (domain.User, error) {
	return r.user, nil
}

type schoolStub struct{ current school.Context }

func (s schoolStub) Current() school.Context { return s.current }
func (s schoolStub) Get(id string) (school.Context, bool) {
	return s.current, id == s.current.ID
}

func TestDevLoginStoresTokenPairAtomically(t *testing.T) {
	repository := &repositorySpy{user: domain.User{ID: "u1", SchoolID: "whut"}}
	service := NewService(repository, schoolStub{school.Context{ID: "whut", Name: "武汉理工大学"}}, time.Hour, 24*time.Hour)
	pair, err := service.DevLogin(context.Background(), "tester", "同学")
	if err != nil {
		t.Fatal(err)
	}
	if repository.storedPairs != 1 || pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("expected one atomic token pair write, got pair=%#v writes=%d", pair, repository.storedPairs)
	}
}

func TestRefreshDelegatesAtomicRotation(t *testing.T) {
	repository := &repositorySpy{user: domain.User{ID: "u1", SchoolID: "whut"}}
	service := NewService(repository, schoolStub{school.Context{ID: "whut", Name: "武汉理工大学"}}, time.Hour, 24*time.Hour)
	pair, err := service.Refresh(context.Background(), "old-refresh-token")
	if err != nil {
		t.Fatal(err)
	}
	if repository.rotations != 1 || repository.storedPairs != 0 || pair.User.SchoolName != "武汉理工大学" {
		t.Fatalf("refresh must use one rotation transaction: pair=%#v rotations=%d", pair, repository.rotations)
	}
}
