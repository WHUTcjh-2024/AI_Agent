package llm

import (
	"context"
	"strings"
	"unicode/utf8"
)

type MockProvider struct {
	model string
}

func NewMockProvider(model string) *MockProvider {
	if strings.TrimSpace(model) == "" {
		model = "asku-mock"
	}
	return &MockProvider{model: model}
}

func (p *MockProvider) Name() string { return "mock" }

func (p *MockProvider) Generate(ctx context.Context, request Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	model := request.Model
	if model == "" {
		model = p.model
	}
	content := "AskU 的 LLM Gateway 已完成联调。\n\n当前使用 Mock Provider 验证模型适配、用量统计和错误隔离；尚未接入真实校园知识与正式大模型，因此不会生成未经核验的学校政策。"
	if len(request.Messages) > 0 && strings.Contains(request.Messages[0].Content, "引用由后端根据真实证据统一生成") {
		content = "已找到与问题相关的学校官方资料。\n\n当前使用 Mock Provider，不归纳具体政策、日期或条件；请通过下方引用核对学校原文。"
	}
	return Response{
		Content: content,
		Model:   model,
		Usage: Usage{
			InputTokens:  estimateMessages(request.Messages),
			OutputTokens: estimateTokens(content),
			Estimated:    true,
		},
	}, nil
}

func (p *MockProvider) Stream(ctx context.Context, request Request) (<-chan StreamEvent, error) {
	response, err := p.Generate(ctx, request)
	if err != nil {
		return nil, err
	}
	events := make(chan StreamEvent, 4)
	go func() {
		defer close(events)
		parts := semanticChunks(response.Content)
		for _, part := range parts {
			select {
			case <-ctx.Done():
				events <- StreamEvent{Err: ctx.Err()}
				return
			case events <- StreamEvent{Delta: part}:
			}
		}
		select {
		case <-ctx.Done():
			events <- StreamEvent{Err: ctx.Err()}
		case events <- StreamEvent{Response: &response}:
		}
	}()
	return events, nil
}

func estimateMessages(messages []Message) int {
	total := 0
	for _, message := range messages {
		total += estimateTokens(message.Content) + 4
	}
	return total
}

func estimateTokens(text string) int {
	runes := utf8.RuneCountInString(text)
	if runes == 0 {
		return 0
	}
	return (runes + 2) / 3
}

func semanticChunks(content string) []string {
	paragraphs := strings.Split(content, "\n\n")
	chunks := make([]string, 0, len(paragraphs))
	for index, paragraph := range paragraphs {
		if index < len(paragraphs)-1 {
			paragraph += "\n\n"
		}
		if paragraph != "" {
			chunks = append(chunks, paragraph)
		}
	}
	return chunks
}
