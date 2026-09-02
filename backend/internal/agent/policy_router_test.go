package agent

import (
	"context"
	"testing"
)

func TestPolicyRouterUsesKnowledgeForStableCampusQuestion(t *testing.T) {
	plan, err := NewPolicyRouter().Plan(context.Background(), Request{Question: "奖学金怎么评？"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Route != "knowledge" || plan.Knowledge == nil || plan.Search != nil {
		t.Fatalf("stable question must route to knowledge: %#v", plan)
	}
}

func TestPolicyRouterUsesHybridRetrievalForFreshQuestion(t *testing.T) {
	plan, err := NewPolicyRouter().Plan(context.Background(), Request{Question: "今年四六级什么时候报名？"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Route != RouteHybrid || plan.Search == nil || plan.Knowledge == nil {
		t.Fatalf("fresh question must route to knowledge plus web search: %#v", plan)
	}
}

func TestPolicyRouterUsesWebForDocumentedIntegrationProbe(t *testing.T) {
	plan, err := NewPolicyRouter().Plan(context.Background(), Request{Question: "官网搜索测试"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Route != "web_search" || plan.Search == nil || plan.Knowledge != nil || plan.Reason != "integration_probe_requires_official_search" {
		t.Fatalf("documented integration probe must route to web search: %#v", plan)
	}
}

func TestPolicyRouterHandlesProductIntroductionWithoutExternalCall(t *testing.T) {
	plan, err := NewPolicyRouter().Plan(context.Background(), Request{Question: "你能做什么？"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Route != "controlled" || plan.Answer == "" || plan.Knowledge != nil || plan.Search != nil {
		t.Fatalf("product introduction must be controlled: %#v", plan)
	}
}
