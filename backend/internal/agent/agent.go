package agent

import (
	"context"
	"strings"
	"time"

	"asku/backend/internal/domain"
	"asku/backend/internal/knowledge"
	"asku/backend/internal/llm"
	"asku/backend/internal/websearch"
)

const (
	RouteControlled = "controlled"
	RouteLLM        = "llm"
	RouteKnowledge  = "knowledge"
	RouteWebSearch  = "web_search"
	RouteHybrid     = "hybrid"
)

type Plan struct {
	Answer            string
	Sources           []domain.Source
	Chunks            []string
	Fail              bool
	Generation        *llm.Request
	Knowledge         *knowledge.Request
	Search            *websearch.Request
	KnowledgeEvidence []knowledge.Evidence
	SearchEvidence    []websearch.Evidence
	Route             string
	Reason            string
}

type Router interface {
	Plan(ctx context.Context, request Request) (Plan, error)
}

type Request struct {
	Question string
}

// MockRouter is the controlled routing adapter used before the real policy
// orchestrator is connected. It never invents policy details without sources.
type MockRouter struct{}

func NewMockRouter() *MockRouter { return &MockRouter{} }

func (m *MockRouter) Plan(ctx context.Context, request Request) (Plan, error) {
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	question := request.Question
	normalized := strings.ToLower(strings.TrimSpace(question))
	if strings.Contains(normalized, "offline") || strings.Contains(question, "网络错误") {
		return Plan{Fail: true, Route: RouteControlled, Reason: "mock_failure_test"}, nil
	}
	if strings.Contains(normalized, "no-source") || strings.Contains(question, "养宠物") || strings.Contains(question, "可靠来源") {
		answer := "暂时没有找到可靠的学校官方信息。\n\n你可以换一种方式提问，或查看学校相关部门发布的原始资料。"
		return Plan{Answer: answer, Chunks: ChunkAnswer(answer), Route: RouteControlled, Reason: "no_reliable_source"}, nil
	}
	if strings.Contains(normalized, "web-search") || strings.Contains(question, "官网搜索") || needsFreshWebSearch(question) {
		return Plan{
			Search: &websearch.Request{Query: question}, Route: RouteWebSearch, Reason: "freshness_requires_official_search",
		}, nil
	}
	if strings.Contains(question, "转专业") {
		answer := "当前校园政策知识库尚未接入，因此 AskU 不提供未经核验的具体日期。\n\n## 建议确认的内容\n\n1. 本年度学校正式通知的发布时间；\n2. 目标学院的接收条件与名额；\n3. 申请入口、材料和截止时间。\n\n请通过下方学校官方入口查看最新通知。"
		sources := []domain.Source{
			{
				ID: "src_dev_whut_undergraduate", Title: "武汉理工大学本科生院（联调入口）", Publisher: "武汉理工大学本科生院",
				Department: "武汉理工大学本科生院", PublishedAt: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
				Audience: "本科生", Summary: "联调用来源，便于验证来源展示和原文跳转；不代表具体政策通知。",
				URL: "https://jwc.whut.edu.cn/", OfficialURL: "https://jwc.whut.edu.cn/", Official: true,
				SourceType: "OFFICIAL_WEB", DocumentType: "HTML", Authority: "OFFICIAL_DEPARTMENT",
				Attachments: []domain.Attachment{}, Evidence: []string{},
			},
		}
		return Plan{Answer: answer, Sources: sources, Chunks: ChunkAnswer(answer), Route: RouteControlled, Reason: "verified_fixture"}, nil
	}
	generation := llm.Request{Messages: []llm.Message{
		{Role: llm.RoleSystem, Content: "你是 AskU 联调助手。校园知识库尚未接入；不得编造学校政策、日期或流程，只说明当前可核验的信息边界。"},
		{Role: llm.RoleUser, Content: question},
	}}
	return Plan{Generation: &generation, Route: RouteLLM, Reason: "no_retrieval_required_for_integration"}, nil
}

func needsFreshWebSearch(question string) bool {
	return NewQuestionAnalyzer(nil).Analyze(question).Freshness == FreshnessCurrent
}

func ChunkAnswer(answer string) []string {
	parts := strings.Split(answer, "\n\n")
	chunks := make([]string, 0, len(parts))
	for index, part := range parts {
		if index < len(parts)-1 {
			part += "\n\n"
		}
		if part != "" {
			chunks = append(chunks, part)
		}
	}
	return chunks
}
