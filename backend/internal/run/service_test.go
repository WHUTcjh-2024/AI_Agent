package run

import (
	"context"
	"testing"

	"asku/backend/internal/domain"
)

type finalizationRepository struct {
	finalized bool
	status    string
}

func (r *finalizationRepository) CreateUserMessageAndRun(context.Context, string, domain.Message) (domain.Message, domain.AgentRun, error) {
	return domain.Message{}, domain.AgentRun{}, nil
}
func (r *finalizationRepository) CompleteAssistantMessage(context.Context, string, domain.Message, []domain.Source) (domain.Message, error) {
	return domain.Message{}, nil
}
func (r *finalizationRepository) RunOwner(context.Context, string) (string, string, string, error) {
	return "owner", "session", "GENERATING", nil
}
func (r *finalizationRepository) UpdateRunStatus(context.Context, string, string, string) error {
	return nil
}
func (r *finalizationRepository) FinalizeRun(_ context.Context, runID, status, _, eventType string, _ any) (domain.RunEvent, bool, error) {
	if r.finalized {
		return domain.RunEvent{}, false, nil
	}
	r.finalized = true
	r.status = status
	return domain.RunEvent{RunID: runID, Sequence: 1, Type: eventType}, true, nil
}
func (r *finalizationRepository) UpsertSource(context.Context, string, domain.Source) error {
	return nil
}
func (r *finalizationRepository) AppendRunEvent(context.Context, string, string, any) (domain.RunEvent, error) {
	return domain.RunEvent{}, nil
}

func TestCancelWithoutLiveWorkerPersistsAndPublishesTerminalEvent(t *testing.T) {
	repository := &finalizationRepository{}
	hub := NewHub()
	service := NewService(repository, nil, hub, false)
	events, unsubscribe := hub.Subscribe("run_1")
	defer unsubscribe()
	if err := service.Cancel(context.Background(), "owner", "run_1"); err != nil {
		t.Fatal(err)
	}
	event := <-events
	if repository.status != "CANCELLED" || event.Type != "run.failed" {
		t.Fatalf("cancel must persist and publish one terminal event: status=%s event=%#v", repository.status, event)
	}
	if err := service.finalize(context.Background(), "run_1", "FAILED", "late", "run.failed", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	select {
	case duplicate := <-events:
		t.Fatalf("terminal transition must be idempotent, got %#v", duplicate)
	default:
	}
}
