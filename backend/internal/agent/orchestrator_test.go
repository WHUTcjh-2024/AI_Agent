package agent

import (
	"context"
	"reflect"
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
	return websearch.Response{Evidence: []websearch.Evidence{{ID: "src_1", Title: "官方通知", URL: "https://whut.edu.cn/", Publisher: "武汉理工大学", PublishedAt: time.Now(), Official: true}}, Stats: websearch.Stats{PagesFetched: 1}}, nil
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
}

type answerCacheStub struct{ answer CachedAnswer }

func (c answerCacheStub) Lookup(context.Context, string, string) (CachedAnswer, bool, error) {
	return c.answer, true, nil
}

func TestAnswerCacheHitBypassesExternalCapabilities(t *testing.T) {
	capabilities := testCapabilities(nil)
	capabilities.AnswerCache = answerCacheStub{answer: CachedAnswer{Answer: "缓存中的已核验答案"}}
	orchestrator, err := NewOrchestrator(NewPolicyRouter(), capabilities)
	if err != nil {
		t.Fatal(err)
	}
	progress := &progressRecorder{}
	result, err := orchestrator.Execute(context.Background(), ExecutionRequest{SchoolID: "whut", Question: "校历在哪里？"}, progress)
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
