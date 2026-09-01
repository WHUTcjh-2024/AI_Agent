package llm

import (
	"context"
	"errors"
	"sync"
	"testing"

	"asku/backend/internal/domain"
)

type memoryRecorder struct {
	mu      sync.Mutex
	records []domain.UsageRecord
}

func (r *memoryRecorder) RecordUsage(_ context.Context, record domain.UsageRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record)
	return nil
}

type failingProvider struct{}

func (failingProvider) Name() string { return "failing" }
func (failingProvider) Generate(context.Context, Request) (Response, error) {
	return Response{}, &ProviderError{Code: "rate_limited", Retryable: true}
}
func (failingProvider) Stream(context.Context, Request) (<-chan StreamEvent, error) {
	return nil, &ProviderError{Code: "rate_limited", Retryable: true}
}

func TestGatewayRecordsUsageAndCost(t *testing.T) {
	recorder := &memoryRecorder{}
	gateway := NewGateway(NewMockProvider("mock-v1"), recorder, Pricing{
		InputRMBPerMillionTokens: 2, OutputRMBPerMillionTokens: 8,
	})
	response, err := gateway.Generate(context.Background(), CallContext{UserID: "usr_1", RunID: "run_1"}, Request{
		Messages: []Message{{Role: RoleUser, Content: "测试"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Content == "" || response.Usage.InputTokens == 0 || response.Usage.OutputTokens == 0 {
		t.Fatalf("invalid response: %#v", response)
	}
	if len(recorder.records) != 1 {
		t.Fatalf("expected one usage record, got %d", len(recorder.records))
	}
	record := recorder.records[0]
	expectedCost := int64(response.Usage.InputTokens*2 + response.Usage.OutputTokens*8)
	if record.Status != "succeeded" || record.EstimatedCostMicroRMB != expectedCost || !record.TokensEstimated {
		t.Fatalf("unexpected usage record: %#v", record)
	}
}

func TestGatewayRecordsProviderFailure(t *testing.T) {
	recorder := &memoryRecorder{}
	gateway := NewGateway(failingProvider{}, recorder, Pricing{})
	_, err := gateway.Generate(context.Background(), CallContext{UserID: "usr_1"}, Request{})
	if err == nil {
		t.Fatal("expected provider failure")
	}
	if len(recorder.records) != 1 || recorder.records[0].Status != "failed" || recorder.records[0].ErrorCode != "rate_limited" {
		t.Fatalf("failure usage was not recorded correctly: %#v", recorder.records)
	}
}

func TestGatewayStreamReconstructsAnswerAndRecordsOnce(t *testing.T) {
	recorder := &memoryRecorder{}
	gateway := NewGateway(NewMockProvider("mock-v1"), recorder, Pricing{})
	events, err := gateway.Stream(context.Background(), CallContext{UserID: "usr_1", RunID: "run_1"}, Request{})
	if err != nil {
		t.Fatal(err)
	}
	var content string
	var final *Response
	for event := range events {
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		content += event.Delta
		if event.Response != nil {
			copy := *event.Response
			final = &copy
		}
	}
	if final == nil || content != final.Content {
		t.Fatalf("stream did not reconstruct response: %q %#v", content, final)
	}
	if len(recorder.records) != 1 {
		t.Fatalf("stream must record exactly once, got %d", len(recorder.records))
	}
}

func TestProviderErrorSupportsErrorsAs(t *testing.T) {
	err := error(&ProviderError{Code: "test"})
	var target *ProviderError
	if !errors.As(err, &target) || target.Code != "test" {
		t.Fatal("typed provider errors must remain inspectable")
	}
}
