package agent

import (
	"context"
	"strings"
	"testing"
)

func TestTransferAnswerDoesNotInventPolicyDate(t *testing.T) {
	result, err := NewMockRouter().Plan(context.Background(), Request{Question: "转专业有什么要求？"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) != 1 || !result.Sources[0].Official {
		t.Fatalf("expected one official integration source, got %#v", result.Sources)
	}
	if strings.Contains(result.Answer, "5 月") || !strings.Contains(result.Answer, "不提供未经核验的具体日期") {
		t.Fatalf("answer must be explicit about the unverified policy boundary: %s", result.Answer)
	}
	if strings.Join(result.Chunks, "") != result.Answer {
		t.Fatal("semantic chunks must reconstruct the complete answer")
	}
}

func TestFreshQuestionRoutesToWebSearch(t *testing.T) {
	result, err := NewMockRouter().Plan(context.Background(), Request{Question: "四六级什么时候报名？"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Search == nil || result.Route != "web_search" {
		t.Fatalf("fresh question must use web search plan: %#v", result)
	}
}

func TestNoReliableSourceScenario(t *testing.T) {
	result, err := NewMockRouter().Plan(context.Background(), Request{Question: "宿舍可以养宠物吗？"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) != 0 {
		t.Fatalf("unverified answer must not attach sources: %#v", result.Sources)
	}
	if !strings.Contains(result.Answer, "暂时没有找到可靠") {
		t.Fatalf("expected honest no-source response, got %s", result.Answer)
	}
}

func TestNetworkFailureScenario(t *testing.T) {
	result, err := NewMockRouter().Plan(context.Background(), Request{Question: "offline"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fail {
		t.Fatal("offline keyword should exercise the failure path")
	}
}

func TestGenericQuestionReturnsGenerationContract(t *testing.T) {
	result, err := NewMockRouter().Plan(context.Background(), Request{Question: "测试 LLM Gateway"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation == nil || len(result.Generation.Messages) != 2 {
		t.Fatalf("expected a decoupled LLM request, got %#v", result.Generation)
	}
	if result.Answer != "" {
		t.Fatal("agent routing must not generate the provider response itself")
	}
}

func TestExplicitWebSearchReturnsProviderIndependentPlan(t *testing.T) {
	result, err := NewMockRouter().Plan(context.Background(), Request{Question: "官网搜索测试"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Search == nil || result.Search.Query != "官网搜索测试" {
		t.Fatalf("unexpected search plan: %#v", result.Search)
	}
	if result.Route != "web_search" || result.Sources != nil {
		t.Fatalf("agent must return a search plan, not provider results: %#v", result)
	}
}
