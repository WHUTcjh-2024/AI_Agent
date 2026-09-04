package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"asku/backend/internal/knowledge"
	"asku/backend/internal/websearch"
)

type capturingKnowledge struct{ request knowledge.Request }

func (s *capturingKnowledge) Search(ctx context.Context, request knowledge.Request) (knowledge.Response, error) {
	s.request = request
	return (knowledgeStub{}).Search(ctx, request)
}

type capturingWeb struct{ request websearch.Request }

func (s *capturingWeb) Gather(ctx context.Context, request websearch.Request) (websearch.Response, error) {
	s.request = request
	return (searchStub{}).Gather(ctx, request)
}

func TestEffectiveRetrievalPreservesOriginalGenerationAndSchoolScope(t *testing.T) {
	const original = "您好！麻烦问一下，我想请教一下今年四六级什么时候报名？"
	const effective = "今年四六级什么时候报名？"
	kg, web, generator := &capturingKnowledge{}, &capturingWeb{}, &capturingGenerator{}
	capabilities := testCapabilities(generator)
	capabilities.Knowledge, capabilities.WebSearch = kg, web
	orchestrator, err := NewOrchestrator(NewPolicyRouter(), capabilities)
	if err != nil {
		t.Fatal(err)
	}
	_, err = orchestrator.Execute(context.Background(), ExecutionRequest{SchoolID: "testu", RunID: "run-test", Question: original}, &progressRecorder{})
	if err != nil {
		t.Fatal(err)
	}
	if kg.request.Query != effective || web.request.Query != effective || kg.request.SchoolID != "testu" || web.request.SchoolID != "testu" || kg.request.RunID != "run-test" {
		t.Fatalf("lost query/scope: %+v %+v", kg.request, web.request)
	}
	found := false
	for _, message := range generator.request.Messages {
		if strings.Contains(message.Content, original) {
			found = true
		}
	}
	if !found {
		t.Fatal("generation lost original user question")
	}
}

func TestFreshLanguageNeverReadsStableAnswerCache(t *testing.T) {
	router := NewPolicyRouterWithAnalyzer(NewQuestionAnalyzer(func() time.Time { return time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC) }))
	for _, question := range []string{"你好，请问四六级什么时候报名？", "嗨，现在还能选课吗？", "2024 年政策现在还有效吗", "2026-2027 学年校历", "你好像知道今年什么时候报名"} {
		t.Run(question, func(t *testing.T) {
			cache := &countingAnswerCache{}
			capabilities := testCapabilities(generatorStub{})
			capabilities.AnswerCache = cache
			orchestrator, err := NewOrchestrator(router, capabilities)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = orchestrator.Execute(context.Background(), ExecutionRequest{SchoolID: "testu", Question: question}, &progressRecorder{}); err != nil {
				t.Fatal(err)
			}
			if cache.lookups != 0 || cache.stores != 0 {
				t.Fatal("fresh question accessed stable cache")
			}
		})
	}
}

func TestExplicitYearUsesInjectedClock(t *testing.T) {
	for _, c := range []struct {
		year  int
		route string
	}{{2026, RouteHybrid}, {2028, RouteKnowledge}} {
		router := NewPolicyRouterWithAnalyzer(NewQuestionAnalyzer(func() time.Time { return time.Date(c.year, 1, 1, 0, 0, 0, 0, time.UTC) }))
		plan, err := router.Plan(context.Background(), Request{Question: "2026 年转专业时间"})
		if err != nil || plan.Route != c.route {
			t.Fatalf("year=%d plan=%+v err=%v", c.year, plan, err)
		}
	}
}
