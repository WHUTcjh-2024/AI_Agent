package llm

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type poolProviderStub struct {
	mu       sync.Mutex
	failures map[string]error
	calls    []string
}

func (p *poolProviderStub) Name() string { return "pool-stub" }

func (p *poolProviderStub) Generate(_ context.Context, request Request) (Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, request.Model)
	if err := p.failures[request.Model]; err != nil {
		return Response{}, err
	}
	return Response{Content: "ok", Model: request.Model}, nil
}

func (p *poolProviderStub) Stream(ctx context.Context, request Request) (<-chan StreamEvent, error) {
	response, err := p.Generate(ctx, request)
	if err != nil {
		return nil, err
	}
	events := make(chan StreamEvent, 1)
	events <- StreamEvent{Response: &response}
	close(events)
	return events, nil
}

func quotaExhausted() error {
	return &ProviderError{Code: "AllocationQuota.FreeTierOnly", StatusCode: 403}
}

func TestModelPoolAdvancesOnceAndStaysOnWorkingModel(t *testing.T) {
	provider := &poolProviderStub{failures: map[string]error{"first": quotaExhausted()}}
	pool, err := NewModelPoolProvider(provider, []string{"first", "second", "third"})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		response, callErr := pool.Generate(context.Background(), Request{})
		if callErr != nil || response.Model != "second" {
			t.Fatalf("unexpected response: %#v %v", response, callErr)
		}
	}
	if got := provider.calls; len(got) != 3 || got[0] != "first" || got[1] != "second" || got[2] != "second" {
		t.Fatalf("unexpected model attempts: %#v", got)
	}
}

func TestModelPoolConcurrentFailuresDoNotSkipNextModel(t *testing.T) {
	provider := &poolProviderStub{failures: map[string]error{"first": quotaExhausted()}}
	pool, err := NewModelPoolProvider(provider, []string{"first", "second", "third"})
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			response, callErr := pool.Generate(context.Background(), Request{})
			if callErr != nil || response.Model != "second" {
				t.Errorf("unexpected response: %#v %v", response, callErr)
			}
		}()
	}
	wait.Wait()
	provider.mu.Lock()
	defer provider.mu.Unlock()
	for _, model := range provider.calls {
		if model == "third" {
			t.Fatal("concurrent quota failures skipped the next model")
		}
	}
}

func TestModelPoolDoesNotRotateOnAuthenticationFailure(t *testing.T) {
	provider := &poolProviderStub{failures: map[string]error{"first": &ProviderError{Code: "invalid_api_key", StatusCode: 401}}}
	pool, err := NewModelPoolProvider(provider, []string{"first", "second"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Generate(context.Background(), Request{}); err == nil {
		t.Fatal("expected authentication failure")
	}
	if len(provider.calls) != 1 || provider.calls[0] != "first" {
		t.Fatalf("authentication failure must not rotate models: %#v", provider.calls)
	}
}

func TestModelPoolReturnsTerminalErrorAfterAllModelsExpire(t *testing.T) {
	provider := &poolProviderStub{failures: map[string]error{"first": quotaExhausted(), "second": quotaExhausted()}}
	pool, err := NewModelPoolProvider(provider, []string{"first", "second"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Stream(context.Background(), Request{})
	var providerError *ProviderError
	if err == nil || !errors.As(err, &providerError) || providerError.Code != "model_pool_exhausted" {
		t.Fatalf("unexpected terminal error: %v", err)
	}
}
