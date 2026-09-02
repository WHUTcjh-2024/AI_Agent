package agent

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"asku/backend/internal/domain"
	"asku/backend/internal/knowledge"
	"asku/backend/internal/llm"
	"asku/backend/internal/websearch"
)

type progressRecorder struct {
	events   []string
	deltas   []string
	sources  []domain.Source
	metadata map[string]any
}

func (p *progressRecorder) RouteResolved(context.Context, string, string) error {
	p.events = append(p.events, "route")
	return nil
}
func (p *progressRecorder) RetrievalStarted(context.Context, string) error {
	p.events = append(p.events, "retrieval")
	return nil
}
func (p *progressRecorder) RetrievalCompleted(context.Context, string, int, map[string]any) error {
	p.events = append(p.events, "retrieval_completed")
	return nil
}
func (p *progressRecorder) SourcesUpdated(_ context.Context, sources []domain.Source, metadata map[string]any) error {
	p.events = append(p.events, "sources")
	p.sources = sources
	p.metadata = metadata
	return nil
}
func (p *progressRecorder) GenerationStarted(context.Context, string) error {
	p.events = append(p.events, "generation")
	return nil
}
func (p *progressRecorder) MessageDelta(_ context.Context, delta string) error {
	p.events = append(p.events, "delta")
	p.deltas = append(p.deltas, delta)
	return nil
}

type searchStub struct{}

func (searchStub) Gather(context.Context, websearch.Request) (websearch.Response, error) {
	return websearch.Response{Evidence: []websearch.Evidence{{
		ID: "src_1", Title: "官方通知", URL: "https://whut.edu.cn/", Publisher: "武汉理工大学",
		PublishedAt: time.Now(), Excerpt: "学校官网发布的可核验证据。", Official: true,
	}}, Stats: websearch.Stats{PagesFetched: 1}}, nil
}

type generatorStub struct{}

func (generatorStub) ProviderName() string { return "test-llm" }
func (generatorStub) Generate(context.Context, llm.CallContext, llm.Request) (llm.Response, error) {
	panic("orchestrator must use the streaming generator path")
}
func (generatorStub) Stream(context.Context, llm.CallContext, llm.Request) (<-chan llm.StreamEvent, error) {
	events := make(chan llm.StreamEvent, 2)
	events <- llm.StreamEvent{Delta: "模型"}
	events <- llm.StreamEvent{Delta: "回答", Response: &llm.Response{Content: "模型回答"}}
	close(events)
	return events, nil
}

func testCapabilities(generator llm.Generator) Capabilities {
	return Capabilities{
		Generator: generator, Knowledge: knowledge.NewDisabledSearcher(), WebSearch: searchStub{},
		SearchTopN: 3, KnowledgeTopN: 4,
	}
}

func TestOrchestratorOwnsSearchCompositionAndProgressOrder(t *testing.T) {
	orchestrator, err := NewOrchestrator(NewMockRouter(), testCapabilities(generatorStub{}))
	if err != nil {
		t.Fatal(err)
	}
	progress := &progressRecorder{}
	result, err := orchestrator.Execute(context.Background(), ExecutionRequest{SchoolID: "whut", Question: "最新校历"}, progress)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(progress.events, []string{"route", "retrieval", "retrieval_completed", "sources", "generation", "delta"}) {
		t.Fatalf("unexpected progress: %#v", progress.events)
	}
	if len(result.Sources) != 1 || len(progress.sources) != 1 || progress.metadata["searchStats"] == nil {
		t.Fatalf("search result was not normalized: %#v", result)
	}
	if result.Answer == "" || len(progress.deltas) == 0 {
		t.Fatal("orchestrator must stream the controlled answer")
	}
	if len(result.Citations) != 1 || result.Citations[0].Index != 1 || result.Citations[0].SourceID != result.Sources[0].ID {
		t.Fatalf("web evidence must produce a structured citation: %#v", result.Citations)
	}
}

func TestOrchestratorOwnsLLMComposition(t *testing.T) {
	orchestrator, err := NewOrchestrator(NewMockRouter(), testCapabilities(generatorStub{}))
	if err != nil {
		t.Fatal(err)
	}
	progress := &progressRecorder{}
	result, err := orchestrator.Execute(context.Background(), ExecutionRequest{UserID: "u", RunID: "r", Question: "普通联调问题"}, progress)
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer != "模型回答" || !reflect.DeepEqual(progress.deltas, []string{"模型回答"}) {
		t.Fatalf("unexpected generation result: %#v", result)
	}
}

type nilStreamGenerator struct{ generatorStub }

func (nilStreamGenerator) Stream(context.Context, llm.CallContext, llm.Request) (<-chan llm.StreamEvent, error) {
	return nil, nil
}

func TestOrchestratorRejectsNilProviderStream(t *testing.T) {
	orchestrator, err := NewOrchestrator(NewMockRouter(), testCapabilities(nilStreamGenerator{}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = orchestrator.Execute(context.Background(), ExecutionRequest{Question: "普通联调问题"}, &progressRecorder{})
	if err == nil {
		t.Fatal("nil provider stream must fail instead of blocking the run")
	}
}

type knowledgeStub struct{}

func (knowledgeStub) Search(context.Context, knowledge.Request) (knowledge.Response, error) {
	publishedAt := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	return knowledge.Response{
		Evidence: []knowledge.Evidence{{
			ChunkID: "chunk-1", KnowledgeID: "knowledge-1", SourceID: "src_kb_1", Title: "奖学金评定办法",
			Content: "奖学金评定以学校正式办法为准。", SourceURL: "https://whut.edu.cn/rule", Publisher: "武汉理工大学", PublishedAt: &publishedAt,
		}},
		Stats: knowledge.Stats{Provider: "test", Configured: true, Hits: 1},
	}, nil
}

type capturingGenerator struct {
	request llm.Request
}

func (g *capturingGenerator) ProviderName() string { return "capturing-llm" }
func (g *capturingGenerator) Generate(context.Context, llm.CallContext, llm.Request) (llm.Response, error) {
	panic("orchestrator must use the streaming generator path")
}
func (g *capturingGenerator) Stream(_ context.Context, _ llm.CallContext, request llm.Request) (<-chan llm.StreamEvent, error) {
	g.request = request
	events := make(chan llm.StreamEvent, 1)
	events <- llm.StreamEvent{Delta: "混合回答", Response: &llm.Response{Content: "混合回答"}}
	close(events)
	return events, nil
}

func TestHybridAgentCombinesKnowledgeAndFreshOfficialEvidence(t *testing.T) {
	generator := &capturingGenerator{}
	capabilities := testCapabilities(generator)
	capabilities.Knowledge = knowledgeStub{}
	orchestrator, err := NewOrchestrator(NewPolicyRouter(), capabilities)
	if err != nil {
		t.Fatal(err)
	}
	progress := &progressRecorder{}
	result, err := orchestrator.Execute(context.Background(), ExecutionRequest{
		UserID: "user", SchoolID: "whut", RunID: "run", Question: "今年奖学金什么时候评？",
	}, progress)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) != 2 || len(result.Citations) != 2 {
		t.Fatalf("hybrid result must retain both evidence families: %#v", result)
	}
	if progress.metadata["retrievalMode"] != RouteHybrid || progress.metadata["knowledgeStats"] == nil || progress.metadata["searchStats"] == nil {
		t.Fatalf("hybrid retrieval metadata is incomplete: %#v", progress.metadata)
	}
	if len(generator.request.Messages) != 2 || !strings.Contains(generator.request.Messages[0].Content, "时效事实依据") ||
		!strings.Contains(generator.request.Messages[1].Content, "稳定背景依据") || !strings.Contains(generator.request.Messages[1].Content, "学校官网实时检索结果") {
		t.Fatalf("hybrid grounding prompt lost evidence roles: %#v", generator.request)
	}
}

type failingKnowledgeStub struct{}

func (failingKnowledgeStub) Search(context.Context, knowledge.Request) (knowledge.Response, error) {
	return knowledge.Response{}, errors.New("knowledge unavailable")
}

func TestHybridAgentFallsBackToOfficialWebWhenKnowledgeFails(t *testing.T) {
	capabilities := testCapabilities(generatorStub{})
	capabilities.Knowledge = failingKnowledgeStub{}
	orchestrator, err := NewOrchestrator(NewPolicyRouter(), capabilities)
	if err != nil {
		t.Fatal(err)
	}
	progress := &progressRecorder{}
	result, err := orchestrator.Execute(context.Background(), ExecutionRequest{SchoolID: "whut", Question: "今年校历怎么安排？"}, progress)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) != 1 || result.Sources[0].ID != "src_1" {
		t.Fatalf("official web fallback was not retained: %#v", result.Sources)
	}
	if !reflect.DeepEqual(progress.metadata["degradedCapabilities"], []string{RouteKnowledge}) {
		t.Fatalf("knowledge degradation must be observable: %#v", progress.metadata)
	}
}

func TestHybridAgentReportsDisabledKnowledgeAsDegraded(t *testing.T) {
	orchestrator, err := NewOrchestrator(NewPolicyRouter(), testCapabilities(generatorStub{}))
	if err != nil {
		t.Fatal(err)
	}
	progress := &progressRecorder{}
	result, err := orchestrator.Execute(context.Background(), ExecutionRequest{SchoolID: "whut", Question: "今年校历怎么安排？"}, progress)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) != 1 || !reflect.DeepEqual(progress.metadata["degradedCapabilities"], []string{RouteKnowledge}) {
		t.Fatalf("disabled knowledge must fail open observably: result=%#v metadata=%#v", result, progress.metadata)
	}
}

type failingSearchStub struct{}

func (failingSearchStub) Gather(context.Context, websearch.Request) (websearch.Response, error) {
	return websearch.Response{}, errors.New("web search unavailable")
}

func TestHybridAgentDoesNotUseKnowledgeAsFreshEvidenceWhenWebFails(t *testing.T) {
	capabilities := testCapabilities(generatorStub{})
	capabilities.Knowledge = knowledgeStub{}
	capabilities.WebSearch = failingSearchStub{}
	orchestrator, err := NewOrchestrator(NewPolicyRouter(), capabilities)
	if err != nil {
		t.Fatal(err)
	}
	_, err = orchestrator.Execute(context.Background(), ExecutionRequest{SchoolID: "whut", Question: "今年校历怎么安排？"}, &progressRecorder{})
	executionError, ok := err.(*ExecutionError)
	if !ok || executionError.Code != "web_search_provider_error" {
		t.Fatalf("freshness-critical web failure must fail the run: %#v", err)
	}
}

type emptySearchStub struct{}

func (emptySearchStub) Gather(context.Context, websearch.Request) (websearch.Response, error) {
	return websearch.Response{Evidence: []websearch.Evidence{}}, nil
}

type untrustedSearchStub struct{}

func (untrustedSearchStub) Gather(context.Context, websearch.Request) (websearch.Response, error) {
	return websearch.Response{Evidence: []websearch.Evidence{{
		ID: "private", Title: "非公开结果", URL: "http://127.0.0.1/admin", Publisher: "unknown",
		Excerpt: "不得作为校园时效证据。", Official: false,
	}}}, nil
}

func TestHybridAgentReturnsControlledBoundaryWithoutFreshEvidence(t *testing.T) {
	capabilities := testCapabilities(nil)
	capabilities.Knowledge = knowledgeStub{}
	capabilities.WebSearch = emptySearchStub{}
	orchestrator, err := NewOrchestrator(NewPolicyRouter(), capabilities)
	if err != nil {
		t.Fatal(err)
	}
	result, err := orchestrator.Execute(context.Background(), ExecutionRequest{SchoolID: "whut", Question: "今年校历怎么安排？"}, &progressRecorder{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer != noReliableSourceAnswer() || len(result.Sources) != 0 || len(result.Citations) != 0 {
		t.Fatalf("stale knowledge must not masquerade as fresh evidence: %#v", result)
	}
}

func TestHybridAgentRejectsUncitableFreshEvidence(t *testing.T) {
	capabilities := testCapabilities(nil)
	capabilities.Knowledge = knowledgeStub{}
	capabilities.WebSearch = untrustedSearchStub{}
	orchestrator, err := NewOrchestrator(NewPolicyRouter(), capabilities)
	if err != nil {
		t.Fatal(err)
	}
	result, err := orchestrator.Execute(context.Background(), ExecutionRequest{SchoolID: "whut", Question: "今年校历怎么安排？"}, &progressRecorder{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer != noReliableSourceAnswer() || len(result.Sources) != 0 || len(result.Citations) != 0 {
		t.Fatalf("uncitable web evidence must not ground a fresh answer: %#v", result)
	}
}

type countingAnswerCache struct {
	lookups int
	stores  int
}

func (c *countingAnswerCache) Lookup(context.Context, string, string) (CachedAnswer, bool, error) {
	c.lookups++
	return CachedAnswer{}, false, nil
}
func (c *countingAnswerCache) Store(context.Context, string, string, CachedAnswer) error {
	c.stores++
	return nil
}

func TestHybridAgentBypassesStableAnswerCache(t *testing.T) {
	answerCache := &countingAnswerCache{}
	capabilities := testCapabilities(generatorStub{})
	capabilities.Knowledge = knowledgeStub{}
	capabilities.AnswerCache = answerCache
	orchestrator, err := NewOrchestrator(NewPolicyRouter(), capabilities)
	if err != nil {
		t.Fatal(err)
	}
	_, err = orchestrator.Execute(context.Background(), ExecutionRequest{SchoolID: "whut", Question: "今年校历怎么安排？"}, &progressRecorder{})
	if err != nil {
		t.Fatal(err)
	}
	if answerCache.lookups != 0 || answerCache.stores != 0 {
		t.Fatalf("hybrid answers must bypass stable cache: lookups=%d stores=%d", answerCache.lookups, answerCache.stores)
	}
}

func TestPolicyOrchestratorUsesKnowledgeBeforeGeneration(t *testing.T) {
	capabilities := testCapabilities(generatorStub{})
	capabilities.Knowledge = knowledgeStub{}
	orchestrator, err := NewOrchestrator(NewPolicyRouter(), capabilities)
	if err != nil {
		t.Fatal(err)
	}
	progress := &progressRecorder{}
	result, err := orchestrator.Execute(context.Background(), ExecutionRequest{
		UserID: "user", SchoolID: "whut", RunID: "run", Question: "奖学金怎么评？",
	}, progress)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) != 1 || result.Sources[0].ID != "src_kb_1" || !result.Sources[0].Official {
		t.Fatalf("knowledge evidence was not normalized: %#v", result.Sources)
	}
	if result.Answer != "模型回答" || progress.metadata["knowledgeStats"] == nil {
		t.Fatalf("knowledge-grounded generation did not complete: %#v", result)
	}
	if len(result.Citations) != 1 || result.Citations[0].Index != 1 || result.Citations[0].ChunkID != "chunk-1" {
		t.Fatalf("structured citation was not built from retrieval evidence: %#v", result.Citations)
	}
}

type answerCacheStub struct {
	answer CachedAnswer
	stored *CachedAnswer
}

func (c *answerCacheStub) Lookup(context.Context, string, string) (CachedAnswer, bool, error) {
	return c.answer, true, nil
}
func (c *answerCacheStub) Store(_ context.Context, _, _ string, answer CachedAnswer) error {
	c.stored = &answer
	return nil
}

func TestAnswerCacheHitBypassesExternalCapabilities(t *testing.T) {
	capabilities := testCapabilities(nil)
	capabilities.AnswerCache = &answerCacheStub{answer: CachedAnswer{
		Answer: "缓存中的已核验答案", Sources: []domain.Source{{ID: "src-cache", Official: true}},
		Citations: []domain.Citation{{CitationID: "cit-cache", Index: 1, SourceID: "src-cache", Title: "校历", EvidenceText: "校历内容", Authority: "OFFICIAL_DEPARTMENT", OfficialURL: "https://www.whut.edu.cn"}},
	}}
	orchestrator, err := NewOrchestrator(NewPolicyRouter(), capabilities)
	if err != nil {
		t.Fatal(err)
	}
	progress := &progressRecorder{}
	result, err := orchestrator.Execute(context.Background(), ExecutionRequest{SchoolID: "whut", Question: "奖学金怎么评？"}, progress)
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer != "缓存中的已核验答案" || !reflect.DeepEqual(progress.deltas, []string{"缓存中的已核验答案"}) {
		t.Fatalf("cache route did not return cached answer: %#v", result)
	}
	if progress.metadata["cacheHit"] != true {
		t.Fatalf("cache metadata missing: %#v", progress.metadata)
	}
}

func TestKnowledgeAnswerIsStoredAfterGroundedGeneration(t *testing.T) {
	answerCache := &answerCacheStub{}
	capabilities := testCapabilities(generatorStub{})
	capabilities.Knowledge = knowledgeStub{}
	capabilities.AnswerCache = answerCache
	orchestrator, err := NewOrchestrator(NewPolicyRouter(), capabilities)
	if err != nil {
		t.Fatal(err)
	}
	_, err = orchestrator.Execute(context.Background(), ExecutionRequest{
		SchoolID: "whut", Question: "奖学金怎么评？", RunID: "run",
	}, &progressRecorder{})
	if err != nil {
		t.Fatal(err)
	}
	if answerCache.stored == nil || answerCache.stored.Answer != "模型回答" || len(answerCache.stored.Sources) != 1 {
		t.Fatalf("grounded knowledge answer was not cached: %#v", answerCache.stored)
	}
}

type failingAnswerCache struct{}

func (failingAnswerCache) Lookup(context.Context, string, string) (CachedAnswer, bool, error) {
	return CachedAnswer{}, false, errors.New("redis read failed")
}

func (failingAnswerCache) Store(context.Context, string, string, CachedAnswer) error {
	return errors.New("redis write failed")
}

func TestAnswerCacheFailureDoesNotFailGroundedAnswer(t *testing.T) {
	capabilities := testCapabilities(generatorStub{})
	capabilities.Knowledge = knowledgeStub{}
	capabilities.AnswerCache = failingAnswerCache{}
	orchestrator, err := NewOrchestrator(NewPolicyRouter(), capabilities)
	if err != nil {
		t.Fatal(err)
	}
	result, err := orchestrator.Execute(context.Background(), ExecutionRequest{
		SchoolID: "whut", Question: "奖学金怎么评？", RunID: "run",
	}, &progressRecorder{})
	if err != nil || result.Answer != "模型回答" {
		t.Fatalf("cache failure blocked grounded answer: result=%#v err=%v", result, err)
	}
}

func TestControlledAnswerDoesNotPretendToRetrieve(t *testing.T) {
	orchestrator, err := NewOrchestrator(NewPolicyRouter(), testCapabilities(generatorStub{}))
	if err != nil {
		t.Fatal(err)
	}
	progress := &progressRecorder{}
	_, err = orchestrator.Execute(context.Background(), ExecutionRequest{SchoolID: "whut", Question: "你能做什么？"}, progress)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(progress.events, []string{"route", "generation", "delta", "delta"}) {
		t.Fatalf("controlled answer emitted misleading retrieval progress: %#v", progress.events)
	}
}

type routerStub struct{ plan Plan }

func (r routerStub) Plan(context.Context, Request) (Plan, error) { return r.plan, nil }

func TestOrchestratorRejectsAmbiguousRouterPlan(t *testing.T) {
	orchestrator, err := NewOrchestrator(routerStub{plan: Plan{
		Route: "ambiguous", Search: &websearch.Request{Query: "通知"}, Knowledge: &knowledge.Request{Query: "通知"},
	}}, testCapabilities(generatorStub{}))
	if err != nil {
		t.Fatal(err)
	}
	progress := &progressRecorder{}
	_, err = orchestrator.Execute(context.Background(), ExecutionRequest{SchoolID: "whut", Question: "通知"}, progress)
	if err == nil {
		t.Fatal("ambiguous router plan must be rejected")
	}
	executionError, ok := err.(*ExecutionError)
	if !ok || executionError.Code != "invalid_agent_plan" {
		t.Fatalf("unexpected error: %#v", err)
	}
	if len(progress.events) != 0 {
		t.Fatalf("invalid plan must fail before publishing progress: %#v", progress.events)
	}
}
