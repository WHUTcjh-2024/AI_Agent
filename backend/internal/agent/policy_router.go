package agent

import (
	"asku/backend/internal/knowledge"
	"asku/backend/internal/websearch"
	"context"
	"fmt"
	"strings"
)

// PolicyRouter maps an analyzed question to a capability plan.
type PolicyRouter struct{ analyzer *QuestionAnalyzer }

func NewPolicyRouter() *PolicyRouter { return NewPolicyRouterWithAnalyzer(nil) }

func NewPolicyRouterWithAnalyzer(analyzer *QuestionAnalyzer) *PolicyRouter {
	if analyzer == nil {
		analyzer = NewQuestionAnalyzer(nil)
	}
	return &PolicyRouter{analyzer: analyzer}
}

func (r PolicyRouter) Plan(ctx context.Context, request Request) (Plan, error) {
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	if strings.TrimSpace(request.Question) == "" {
		return Plan{}, fmt.Errorf("agent question must not be empty")
	}
	analyzer := r.analyzer
	if analyzer == nil {
		analyzer = NewQuestionAnalyzer(nil)
	}
	p := analyzer.Analyze(request.Question)
	if p.PureSocial || p.ProductIntro {
		return Plan{Answer: "我是 AskU，负责帮助你查找本校的官方信息。\n\n你可以询问选课、转专业、考试报名、奖学金、图书馆、校历和学生事务等问题。没有可靠来源时，我会明确说明暂未找到，而不会猜测学校政策。", Route: RouteControlled, Reason: p.Reason}, nil
	}
	if p.IntegrationProbe {
		return Plan{Search: &websearch.Request{Query: p.EffectiveQuestion}, Route: RouteWebSearch, Reason: p.Reason}, nil
	}
	plan := Plan{Knowledge: &knowledge.Request{Query: p.EffectiveQuestion}, Route: RouteKnowledge, Reason: p.Reason}
	if p.Freshness == FreshnessCurrent {
		plan.Search = &websearch.Request{Query: p.EffectiveQuestion}
		plan.Route = RouteHybrid
	}
	return plan, nil
}
