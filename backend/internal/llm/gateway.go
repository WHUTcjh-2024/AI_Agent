package llm

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"time"

	"asku/backend/internal/domain"
	"asku/backend/internal/id"
)

type Pricing struct {
	InputRMBPerMillionTokens  float64
	OutputRMBPerMillionTokens float64
}

type Gateway struct {
	provider Provider
	recorder UsageRecorder
	pricing  Pricing
}

func NewGateway(provider Provider, recorder UsageRecorder, pricing Pricing) *Gateway {
	return &Gateway{provider: provider, recorder: recorder, pricing: pricing}
}

func (g *Gateway) ProviderName() string { return g.provider.Name() }

func (g *Gateway) Generate(ctx context.Context, call CallContext, request Request) (response Response, err error) {
	startedAt := time.Now()
	response, err = g.provider.Generate(ctx, request)
	g.record(call, response.Model, response.Usage, startedAt, err)
	return response, err
}

func (g *Gateway) Stream(ctx context.Context, call CallContext, request Request) (<-chan StreamEvent, error) {
	startedAt := time.Now()
	upstream, err := g.provider.Stream(ctx, request)
	if err != nil {
		g.record(call, request.Model, Usage{}, startedAt, err)
		return nil, err
	}
	downstream := make(chan StreamEvent)
	go func() {
		defer close(downstream)
		var final Response
		var terminalErr error
		for event := range upstream {
			if event.Response != nil {
				final = *event.Response
			}
			if event.Err != nil {
				terminalErr = event.Err
			}
			select {
			case <-ctx.Done():
				terminalErr = ctx.Err()
				g.record(call, final.Model, final.Usage, startedAt, terminalErr)
				return
			case downstream <- event:
			}
		}
		g.record(call, final.Model, final.Usage, startedAt, terminalErr)
	}()
	return downstream, nil
}

func (g *Gateway) record(call CallContext, model string, usage Usage, startedAt time.Time, callErr error) {
	if g.recorder == nil || call.UserID == "" {
		return
	}
	status := "succeeded"
	code := ""
	if callErr != nil {
		status = "failed"
		code = errorCode(callErr)
		if errors.Is(callErr, context.Canceled) || errors.Is(callErr, context.DeadlineExceeded) {
			status = "cancelled"
			code = "cancelled"
		}
	}
	record := domain.UsageRecord{
		ID: id.New("use"), UserID: call.UserID, RunID: call.RunID,
		Provider: g.provider.Name(), Model: model,
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		EstimatedCostMicroRMB: g.costMicroRMB(usage),
		LatencyMS:             time.Since(startedAt).Milliseconds(), Status: status,
		ErrorCode: code, TokensEstimated: usage.Estimated, CreatedAt: time.Now().UTC(),
	}
	recordContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := g.recorder.RecordUsage(recordContext, record); err != nil {
		slog.Error("record llm usage", "provider", record.Provider, "run_id", record.RunID, "error", err)
	}
}

func (g *Gateway) costMicroRMB(usage Usage) int64 {
	costRMB := (float64(usage.InputTokens)*g.pricing.InputRMBPerMillionTokens +
		float64(usage.OutputTokens)*g.pricing.OutputRMBPerMillionTokens) / 1_000_000
	return int64(math.Round(costRMB * 1_000_000))
}
