package llm

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
)

// ModelPoolProvider keeps one process-wide model cursor. Concurrent requests
// may observe the same exhausted model, but only one of them can advance the
// cursor, so a burst cannot skip models in the configured order.
type ModelPoolProvider struct {
	provider Provider
	models   []string

	mu      sync.RWMutex
	current int
}

func NewModelPoolProvider(provider Provider, models []string) (*ModelPoolProvider, error) {
	if provider == nil {
		return nil, errors.New("model pool provider must not be nil")
	}
	unique := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		unique = append(unique, model)
	}
	if len(unique) == 0 {
		return nil, errors.New("model pool requires at least one model")
	}
	return &ModelPoolProvider{provider: provider, models: unique}, nil
}

func (p *ModelPoolProvider) Name() string { return p.provider.Name() }

func (p *ModelPoolProvider) Generate(ctx context.Context, request Request) (Response, error) {
	var lastErr error
	for {
		index, model, ok := p.candidate()
		if !ok {
			return Response{}, poolExhaustedError(lastErr)
		}
		request.Model = model
		response, err := p.provider.Generate(ctx, request)
		if err == nil {
			return response, nil
		}
		if !modelCannotContinue(err) {
			return Response{}, err
		}
		lastErr = err
		p.advance(index, model, err)
	}
}

func (p *ModelPoolProvider) Stream(ctx context.Context, request Request) (<-chan StreamEvent, error) {
	var lastErr error
	for {
		index, model, ok := p.candidate()
		if !ok {
			return nil, poolExhaustedError(lastErr)
		}
		request.Model = model
		events, err := p.provider.Stream(ctx, request)
		if err == nil {
			return events, nil
		}
		if !modelCannotContinue(err) {
			return nil, err
		}
		lastErr = err
		p.advance(index, model, err)
	}
}

func (p *ModelPoolProvider) candidate() (int, string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.current >= len(p.models) {
		return p.current, "", false
	}
	return p.current, p.models[p.current], true
}

func (p *ModelPoolProvider) advance(index int, model string, cause error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current != index {
		return
	}
	p.current++
	slog.Warn("llm model removed from active pool", "model", model, "reason", errorCode(cause), "remaining_models", len(p.models)-p.current)
}

func poolExhaustedError(cause error) error {
	return &ProviderError{Code: "model_pool_exhausted", Retryable: false, Cause: cause}
}
