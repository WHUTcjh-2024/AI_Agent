package run

import (
	"context"
	"sync"

	"asku/backend/internal/domain"
)

type Hub struct {
	mu          sync.Mutex
	subscribers map[string]map[chan domain.RunEvent]struct{}
	cancels     map[string]context.CancelFunc
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[string]map[chan domain.RunEvent]struct{}), cancels: make(map[string]context.CancelFunc)}
}

func (h *Hub) Subscribe(runID string) (<-chan domain.RunEvent, func()) {
	channel := make(chan domain.RunEvent, 32)
	h.mu.Lock()
	if h.subscribers[runID] == nil {
		h.subscribers[runID] = make(map[chan domain.RunEvent]struct{})
	}
	h.subscribers[runID][channel] = struct{}{}
	h.mu.Unlock()
	return channel, func() {
		h.mu.Lock()
		if subscribers := h.subscribers[runID]; subscribers != nil {
			if _, exists := subscribers[channel]; !exists {
				h.mu.Unlock()
				return
			}
			delete(subscribers, channel)
			close(channel)
			if len(subscribers) == 0 {
				delete(h.subscribers, runID)
			}
		}
		h.mu.Unlock()
	}
}

func (h *Hub) Publish(event domain.RunEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for subscriber := range h.subscribers[event.RunID] {
		select {
		case subscriber <- event:
		default:
			// Disconnect a lagging subscriber instead of silently creating an
			// unrecoverable sequence gap. The client reconnects and replays DB events.
			close(subscriber)
			delete(h.subscribers[event.RunID], subscriber)
		}
	}
	if len(h.subscribers[event.RunID]) == 0 {
		delete(h.subscribers, event.RunID)
	}
}

func (h *Hub) RegisterCancel(runID string, cancel context.CancelFunc) {
	h.mu.Lock()
	h.cancels[runID] = cancel
	h.mu.Unlock()
}

func (h *Hub) UnregisterCancel(runID string) {
	h.mu.Lock()
	delete(h.cancels, runID)
	h.mu.Unlock()
}

func (h *Hub) Cancel(runID string) bool {
	h.mu.Lock()
	cancel, ok := h.cancels[runID]
	h.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}
