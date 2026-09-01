package agent

import (
	"context"
	"reflect"
	"testing"
	"time"

	"asku/backend/internal/domain"
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

func TestOrchestratorOwnsSearchCompositionAndProgressOrder(t *testing.T) {
	orchestrator, err := NewOrchestrator(NewMockRouter(), generatorStub{}, searchStub{}, 3)
	if err != nil {
		t.Fatal(err)
	}
	progress := &progressRecorder{}
	result, err := orchestrator.Execute(context.Background(), ExecutionRequest{SchoolID: "whut", Question: "最新校历"}, progress)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(progress.events, []string{"route", "retrieval", "sources", "generation", "delta", "delta"}) {
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
	orchestrator, err := NewOrchestrator(NewMockRouter(), generatorStub{}, searchStub{}, 3)
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
	orchestrator, err := NewOrchestrator(NewMockRouter(), nilStreamGenerator{}, searchStub{}, 3)
	if err != nil {
		t.Fatal(err)
	}
	_, err = orchestrator.Execute(context.Background(), ExecutionRequest{Question: "普通联调问题"}, &progressRecorder{})
	if err == nil {
		t.Fatal("nil provider stream must fail instead of blocking the run")
	}
}
