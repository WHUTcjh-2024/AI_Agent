package run

import (
	"context"
	"testing"
	"time"

	"asku/backend/internal/domain"
)

func TestHubPublishesAndUnsubscribes(t *testing.T) {
	hub := NewHub()
	events, unsubscribe := hub.Subscribe("run_1")
	hub.Publish(domain.RunEvent{RunID: "run_1", Sequence: 7, Type: "message.delta"})
	select {
	case event := <-events:
		if event.Sequence != 7 {
			t.Fatalf("unexpected event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive event")
	}
	unsubscribe()
	hub.Publish(domain.RunEvent{RunID: "run_1", Sequence: 8})
}

func TestHubCancelsRegisteredRun(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	hub.RegisterCancel("run_1", cancel)
	if !hub.Cancel("run_1") {
		t.Fatal("expected registered run to be cancelled")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("cancel function was not invoked")
	}
	hub.UnregisterCancel("run_1")
	if hub.Cancel("run_1") {
		t.Fatal("unregistered run must not report cancellation")
	}
}

func TestHubDisconnectsLaggingSubscriberWithoutDoubleClose(t *testing.T) {
	hub := NewHub()
	events, unsubscribe := hub.Subscribe("run_slow")
	for sequence := int64(1); sequence <= 33; sequence++ {
		hub.Publish(domain.RunEvent{RunID: "run_slow", Sequence: sequence})
	}
	for range events { /* Drain until the forced disconnect. */
	}
	unsubscribe()
}
