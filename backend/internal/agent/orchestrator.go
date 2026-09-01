package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"asku/backend/internal/domain"
	"asku/backend/internal/llm"
	"asku/backend/internal/websearch"
)

type ExecutionRequest struct {
	UserID   string
	SchoolID string
	RunID    string
	Question string
}

type ExecutionResult struct {
	Answer  string
	Sources []domain.Source
}

type Progress interface {
	RouteResolved(ctx context.Context, route, reason string) error
	RetrievalStarted(ctx context.Context, engine string) error
	SourcesUpdated(ctx context.Context, sources []domain.Source, metadata map[string]any) error
	GenerationStarted(ctx context.Context, provider string) error
	MessageDelta(ctx context.Context, delta string) error
}

type Executor interface {
	Execute(ctx context.Context, request ExecutionRequest, progress Progress) (ExecutionResult, error)
}

type ExecutionError struct {
	Code      string
	Message   string
	Retryable bool
	Cause     error
}

func (e *ExecutionError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Cause)
	}
	return e.Code
}

func (e *ExecutionError) Unwrap() error { return e.Cause }

// Orchestrator owns capability composition. Run lifecycle code only observes
// product progress and persists the final result.
type Orchestrator struct {
	router     Router
	generator  llm.Generator
	searcher   websearch.Searcher
	searchTopN int
}

func NewOrchestrator(router Router, generator llm.Generator, searcher websearch.Searcher, searchTopN int) (*Orchestrator, error) {
	if router == nil {
		return nil, fmt.Errorf("agent router must not be nil")
	}
	if searchTopN < 1 || searchTopN > 5 {
		return nil, fmt.Errorf("agent search Top-N must be between 1 and 5")
	}
	return &Orchestrator{router: router, generator: generator, searcher: searcher, searchTopN: searchTopN}, nil
}

func (o *Orchestrator) Execute(ctx context.Context, request ExecutionRequest, progress Progress) (ExecutionResult, error) {
	plan, err := o.router.Plan(ctx, Request{Question: request.Question})
	if err != nil {
		return ExecutionResult{}, err
	}
	if err := progress.RouteResolved(ctx, plan.Route, plan.Reason); err != nil {
		return ExecutionResult{}, err
	}

	retrievalEngine := "none"
	if plan.Search != nil {
		retrievalEngine = "web-search"
	} else if len(plan.Sources) > 0 {
		retrievalEngine = "controlled-fixture"
	}
	if err := progress.RetrievalStarted(ctx, retrievalEngine); err != nil {
		return ExecutionResult{}, err
	}
	if plan.Fail {
		return ExecutionResult{}, &ExecutionError{Code: "mock_network_error", Message: "联调模拟：Agent 上游连接失败。", Retryable: true}
	}

	metadata := map[string]any{}
	if plan.Search != nil {
		if o.searcher == nil {
			return ExecutionResult{}, &ExecutionError{Code: "web_search_not_configured", Message: "学校官网搜索尚未配置。"}
		}
		plan.Search.SchoolID = request.SchoolID
		plan.Search.TopN = o.searchTopN
		searchResponse, searchErr := o.searcher.Gather(ctx, *plan.Search)
		if searchErr != nil {
			return ExecutionResult{}, &ExecutionError{Code: "web_search_provider_error", Message: "学校官网搜索暂时不可用，请稍后重试。", Retryable: true, Cause: searchErr}
		}
		metadata["searchStats"] = searchResponse.Stats
		plan.Sources = evidenceToSources(searchResponse.Evidence)
		if len(plan.Sources) == 0 {
			plan.Answer = "暂时没有找到可靠的学校官方信息。\n\n你可以换一种方式提问，或稍后查看学校相关部门发布的原始资料。"
		} else {
			plan.Answer = "已查找学校官方网站，并整理出与问题相关的资料入口。\n\n当前环境用于验证检索与来源展示，尚未接入正式政策答案综合；请点击下方来源查看学校原文。"
		}
		plan.Chunks = ChunkAnswer(plan.Answer)
	}
	if plan.Sources == nil {
		plan.Sources = []domain.Source{}
	}
	if err := progress.SourcesUpdated(ctx, plan.Sources, metadata); err != nil {
		return ExecutionResult{}, err
	}

	provider := "controlled-response"
	if plan.Generation != nil && o.generator != nil {
		provider = o.generator.ProviderName()
	}
	if err := progress.GenerationStarted(ctx, provider); err != nil {
		return ExecutionResult{}, err
	}
	if plan.Generation != nil {
		if o.generator == nil {
			return ExecutionResult{}, &ExecutionError{Code: "llm_not_configured", Message: "模型服务尚未配置。"}
		}
		stream, generationErr := o.generator.Stream(ctx, llm.CallContext{UserID: request.UserID, RunID: request.RunID}, *plan.Generation)
		if generationErr != nil {
			return ExecutionResult{}, &ExecutionError{Code: "llm_provider_error", Message: "模型服务暂时不可用，请稍后重试。", Retryable: true, Cause: generationErr}
		}
		if stream == nil {
			return ExecutionResult{}, &ExecutionError{Code: "llm_invalid_stream", Message: "模型服务暂时不可用，请稍后重试。", Retryable: true}
		}
		answer, streamed, streamErr := streamGeneration(ctx, stream, progress)
		if streamErr != nil {
			return ExecutionResult{}, streamErr
		}
		plan.Answer = answer
		if strings.TrimSpace(plan.Answer) == "" {
			return ExecutionResult{}, &ExecutionError{Code: "llm_empty_response", Message: "模型没有返回有效内容，请稍后重试。", Retryable: true}
		}
		if !streamed {
			for _, chunk := range ChunkAnswer(plan.Answer) {
				if err := progress.MessageDelta(ctx, chunk); err != nil {
					return ExecutionResult{}, err
				}
			}
		}
	} else {
		for _, chunk := range ChunkAnswer(plan.Answer) {
			if err := progress.MessageDelta(ctx, chunk); err != nil {
				return ExecutionResult{}, err
			}
		}
	}
	return ExecutionResult{Answer: plan.Answer, Sources: plan.Sources}, nil
}

func streamGeneration(ctx context.Context, stream <-chan llm.StreamEvent, progress Progress) (string, bool, error) {
	const maxBufferedBytes = 96
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	var complete strings.Builder
	var pending strings.Builder
	finalContent := ""
	streamed := false
	flush := func() error {
		if pending.Len() == 0 {
			return nil
		}
		delta := pending.String()
		pending.Reset()
		streamed = true
		return progress.MessageDelta(ctx, delta)
	}
	for {
		select {
		case <-ctx.Done():
			return "", streamed, ctx.Err()
		case <-ticker.C:
			if err := flush(); err != nil {
				return "", streamed, err
			}
		case event, open := <-stream:
			if !open {
				if err := flush(); err != nil {
					return "", streamed, err
				}
				if finalContent != "" {
					return finalContent, streamed, nil
				}
				return complete.String(), streamed, nil
			}
			if event.Err != nil {
				return "", streamed, &ExecutionError{Code: "llm_provider_error", Message: "模型服务暂时不可用，请稍后重试。", Retryable: true, Cause: event.Err}
			}
			if event.Delta != "" {
				complete.WriteString(event.Delta)
				pending.WriteString(event.Delta)
				if pending.Len() >= maxBufferedBytes {
					if err := flush(); err != nil {
						return "", streamed, err
					}
				}
			}
			if event.Response != nil {
				finalContent = event.Response.Content
			}
		}
	}
}

func evidenceToSources(evidence []websearch.Evidence) []domain.Source {
	sources := make([]domain.Source, 0, len(evidence))
	for _, item := range evidence {
		sources = append(sources, domain.Source{
			ID: item.ID, Title: item.Title, Publisher: item.Publisher,
			PublishedAt: item.PublishedAt, Audience: "本校学生",
			Summary: item.Excerpt, URL: item.URL, Official: item.Official,
		})
	}
	return sources
}
