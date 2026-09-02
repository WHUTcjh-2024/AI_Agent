package agent

import (
	"context"
	"fmt"
	"strings"

	"asku/backend/internal/knowledge"
	"asku/backend/internal/websearch"
)

// PolicyRouter decides only which capability should handle a question. It has
// no provider, school configuration or persistence knowledge.
type PolicyRouter struct{}

func NewPolicyRouter() *PolicyRouter { return &PolicyRouter{} }

func (PolicyRouter) Plan(ctx context.Context, request Request) (Plan, error) {
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	question := strings.TrimSpace(request.Question)
	if question == "" {
		return Plan{}, fmt.Errorf("agent question must not be empty")
	}
	if isIntroductionQuestion(question) {
		answer := "我是 AskU，负责帮助你查找本校的官方信息。\n\n你可以询问选课、转专业、考试报名、奖学金、图书馆、校历和学生事务等问题。没有可靠来源时，我会明确说明暂未找到，而不会猜测学校政策。"
		return Plan{Answer: answer, Route: RouteControlled, Reason: "product_introduction"}, nil
	}
	if isWebSearchProbe(question) {
		return Plan{
			Search: &websearch.Request{Query: question}, Route: RouteWebSearch, Reason: "integration_probe_requires_official_search",
		}, nil
	}
	if needsFreshWebSearch(question) {
		return Plan{
			Knowledge: &knowledge.Request{Query: question},
			Search:    &websearch.Request{Query: question},
			Route:     RouteHybrid,
			Reason:    "freshness_requires_knowledge_and_official_search",
		}, nil
	}
	return Plan{
		Knowledge: &knowledge.Request{Query: question}, Route: RouteKnowledge, Reason: "stable_campus_knowledge_first",
	}, nil
}

func isWebSearchProbe(question string) bool {
	normalized := strings.ToLower(strings.TrimSpace(question))
	return strings.Contains(normalized, "官网搜索测试") || strings.Contains(normalized, "web-search")
}

func isIntroductionQuestion(question string) bool {
	normalized := strings.ToLower(strings.TrimSpace(question))
	for _, marker := range []string{"你是谁", "你能做什么", "介绍一下", "asku是什么", "asku 是什么", "hello", "你好"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
