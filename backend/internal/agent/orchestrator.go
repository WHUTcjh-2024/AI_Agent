package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"asku/backend/internal/domain"
	"asku/backend/internal/knowledge"
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
	Answer    string
	Sources   []domain.Source
	Citations []domain.Citation
}

type Progress interface {
	RouteResolved(ctx context.Context, route, reason string) error
	RetrievalStarted(ctx context.Context, engine string) error
	RetrievalCompleted(ctx context.Context, engine string, hits int, metadata map[string]any) error
	SourcesUpdated(ctx context.Context, sources []domain.Source, metadata map[string]any) error
	GenerationStarted(ctx context.Context, provider string) error
	MessageDelta(ctx context.Context, delta string) error
}

type Executor interface {
	Execute(ctx context.Context, request ExecutionRequest, progress Progress) (ExecutionResult, error)
}

type CachedAnswer struct {
	Answer    string            `json:"answer"`
	Sources   []domain.Source   `json:"sources"`
	Citations []domain.Citation `json:"citations"`
}

// AnswerCache keeps Redis mechanics outside the agent while the orchestrator
// owns the product rule that only stable, grounded knowledge answers are stored.
type AnswerCache interface {
	Lookup(ctx context.Context, schoolID, question string) (CachedAnswer, bool, error)
	Store(ctx context.Context, schoolID, question string, answer CachedAnswer) error
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

// Orchestrator owns routing, cache policy and generation. Retrieval adapters
// are composed behind retrievalCoordinator so Run and API stay capability-free.
type Orchestrator struct {
	router      Router
	generator   llm.Generator
	retrieval   *retrievalCoordinator
	answerCache AnswerCache
}

type Capabilities struct {
	Generator     llm.Generator
	Knowledge     knowledge.Searcher
	WebSearch     websearch.Searcher
	AnswerCache   AnswerCache
	SearchTopN    int
	KnowledgeTopN int
}

func NewOrchestrator(router Router, capabilities Capabilities) (*Orchestrator, error) {
	if router == nil {
		return nil, fmt.Errorf("agent router must not be nil")
	}
	if capabilities.SearchTopN < 1 || capabilities.SearchTopN > 5 {
		return nil, fmt.Errorf("agent search Top-N must be between 1 and 5")
	}
	if capabilities.KnowledgeTopN < 1 || capabilities.KnowledgeTopN > 10 {
		return nil, fmt.Errorf("agent knowledge Top-N must be between 1 and 10")
	}
	return &Orchestrator{
		router: router, generator: capabilities.Generator,
		retrieval:   newRetrievalCoordinator(capabilities.Knowledge, capabilities.WebSearch, capabilities.KnowledgeTopN, capabilities.SearchTopN),
		answerCache: capabilities.AnswerCache,
	}, nil
}

func (o *Orchestrator) Execute(ctx context.Context, request ExecutionRequest, progress Progress) (ExecutionResult, error) {
	if progress == nil {
		return ExecutionResult{}, fmt.Errorf("agent progress sink must not be nil")
	}
	plan, err := o.router.Plan(ctx, Request{Question: request.Question})
	if err != nil {
		return ExecutionResult{}, err
	}
	if err := validatePlan(plan); err != nil {
		return ExecutionResult{}, &ExecutionError{Code: "invalid_agent_plan", Message: "Agent 路由结果无效。", Cause: err}
	}

	// Fresh and hybrid questions must never be answered from the stable-answer
	// cache. Routing is intentionally resolved before cache lookup.
	if plan.Route == RouteKnowledge {
		if result, hit, cacheErr := o.lookupCachedAnswer(ctx, request, progress); cacheErr != nil {
			return ExecutionResult{}, cacheErr
		} else if hit {
			return result, nil
		}
	}

	if err := progress.RouteResolved(ctx, plan.Route, plan.Reason); err != nil {
		return ExecutionResult{}, err
	}
	retrievalEngine, retrievalRequired := retrievalEngine(plan)
	if retrievalRequired {
		if err := progress.RetrievalStarted(ctx, retrievalEngine); err != nil {
			return ExecutionResult{}, err
		}
	}
	if plan.Fail {
		return ExecutionResult{}, &ExecutionError{Code: "mock_network_error", Message: "联调模拟：Agent 上游连接失败。", Retryable: true}
	}

	metadata := map[string]any{}
	if plan.Knowledge != nil || plan.Search != nil {
		outcome, retrievalErr := o.retrieval.Retrieve(ctx, request, plan)
		if retrievalErr != nil {
			return ExecutionResult{}, retrievalErr
		}
		metadata = outcome.Metadata
		applyRetrievalOutcome(&plan, request.Question, outcome)
	}
	if plan.Sources == nil {
		plan.Sources = []domain.Source{}
	}
	if retrievalRequired {
		if err := progress.RetrievalCompleted(ctx, retrievalEngine, len(plan.Sources), metadata); err != nil {
			return ExecutionResult{}, err
		}
		if err := progress.SourcesUpdated(ctx, plan.Sources, metadata); err != nil {
			return ExecutionResult{}, err
		}
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
	} else if err := streamControlledAnswer(ctx, plan.Answer, progress); err != nil {
		return ExecutionResult{}, err
	}

	result := ExecutionResult{Answer: plan.Answer, Sources: plan.Sources, Citations: citationsForPlan(plan)}
	if o.answerCache != nil && plan.Route == RouteKnowledge {
		cached := CachedAnswer{Answer: result.Answer, Sources: result.Sources, Citations: result.Citations}
		if isCacheableAnswer(cached) {
			if err := o.answerCache.Store(ctx, request.SchoolID, request.Question, cached); err != nil {
				slog.Warn("write agent answer cache", "school_id", request.SchoolID, "error", err)
			}
		}
	}
	return result, nil
}

func (o *Orchestrator) lookupCachedAnswer(ctx context.Context, request ExecutionRequest, progress Progress) (ExecutionResult, bool, error) {
	if o.answerCache == nil {
		return ExecutionResult{}, false, nil
	}
	cached, hit, err := o.answerCache.Lookup(ctx, request.SchoolID, request.Question)
	if err != nil {
		slog.Warn("read agent answer cache", "school_id", request.SchoolID, "error", err)
		return ExecutionResult{}, false, nil
	}
	if !hit || !isCacheableAnswer(cached) {
		return ExecutionResult{}, false, nil
	}
	if cached.Sources == nil {
		cached.Sources = []domain.Source{}
	}
	metadata := map[string]any{"cacheHit": true}
	steps := []func() error{
		func() error { return progress.RouteResolved(ctx, "cache", "verified_answer_cache_hit") },
		func() error { return progress.RetrievalStarted(ctx, "answer-cache") },
		func() error { return progress.RetrievalCompleted(ctx, "answer-cache", len(cached.Sources), metadata) },
		func() error { return progress.SourcesUpdated(ctx, cached.Sources, metadata) },
		func() error { return progress.GenerationStarted(ctx, "answer-cache") },
		func() error { return streamControlledAnswer(ctx, cached.Answer, progress) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return ExecutionResult{}, false, err
		}
	}
	return ExecutionResult{Answer: cached.Answer, Sources: cached.Sources, Citations: cached.Citations}, true, nil
}

func isCacheableAnswer(answer CachedAnswer) bool {
	if strings.TrimSpace(answer.Answer) == "" || len(answer.Answer) > 64<<10 || len(answer.Sources) == 0 || len(answer.Citations) == 0 {
		return false
	}
	sourceIDs := make(map[string]struct{}, len(answer.Sources))
	for _, source := range answer.Sources {
		if !source.Official || strings.TrimSpace(source.ID) == "" {
			return false
		}
		sourceIDs[source.ID] = struct{}{}
	}
	for position, citation := range answer.Citations {
		if citation.Index != position+1 || strings.TrimSpace(citation.CitationID) == "" || strings.TrimSpace(citation.EvidenceText) == "" {
			return false
		}
		if _, exists := sourceIDs[citation.SourceID]; !exists {
			return false
		}
		if firstNonEmpty(citation.AttachmentURL, citation.OfficialURL, citation.ParentPageURL) == "" {
			return false
		}
	}
	return true
}

func validatePlan(plan Plan) error {
	if strings.TrimSpace(plan.Route) == "" {
		return fmt.Errorf("route must not be empty")
	}
	if plan.Generation != nil && strings.TrimSpace(plan.Answer) != "" {
		return fmt.Errorf("a plan cannot contain both a generated request and a final answer")
	}
	if (plan.Knowledge != nil || plan.Search != nil) && (plan.Generation != nil || strings.TrimSpace(plan.Answer) != "") {
		return fmt.Errorf("retrieval output must be composed by the orchestrator")
	}
	if plan.Fail {
		return nil
	}
	switch plan.Route {
	case RouteControlled:
		if strings.TrimSpace(plan.Answer) == "" {
			return fmt.Errorf("controlled route requires an answer")
		}
	case RouteLLM:
		if plan.Generation == nil || plan.Knowledge != nil || plan.Search != nil {
			return fmt.Errorf("llm route requires generation only")
		}
	case RouteKnowledge:
		if plan.Knowledge == nil || plan.Search != nil {
			return fmt.Errorf("knowledge route requires knowledge retrieval only")
		}
	case RouteWebSearch:
		if plan.Search == nil || plan.Knowledge != nil {
			return fmt.Errorf("web-search route requires web retrieval only")
		}
	case RouteHybrid:
		if plan.Knowledge == nil || plan.Search == nil {
			return fmt.Errorf("hybrid route requires knowledge and web retrieval")
		}
	default:
		return fmt.Errorf("unsupported route %q", plan.Route)
	}
	return nil
}

func streamControlledAnswer(ctx context.Context, answer string, progress Progress) error {
	for _, chunk := range ChunkAnswer(answer) {
		if err := progress.MessageDelta(ctx, chunk); err != nil {
			return err
		}
	}
	return nil
}

func noReliableSourceAnswer() string {
	return "暂时没有找到可靠的学校官方信息。\n\n你可以换一种方式提问，或稍后查看学校相关部门发布的原始资料。"
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

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
