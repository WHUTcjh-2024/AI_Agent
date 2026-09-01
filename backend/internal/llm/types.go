package llm

import (
	"context"

	"asku/backend/internal/domain"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model       string    `json:"model,omitempty"`
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   int       `json:"maxTokens,omitempty"`
}

type Usage struct {
	InputTokens  int  `json:"inputTokens"`
	OutputTokens int  `json:"outputTokens"`
	Estimated    bool `json:"estimated"`
}

type Response struct {
	Content string `json:"content"`
	Model   string `json:"model"`
	Usage   Usage  `json:"usage"`
}

type StreamEvent struct {
	Delta    string
	Response *Response
	Err      error
}

type Provider interface {
	Name() string
	Generate(ctx context.Context, request Request) (Response, error)
	Stream(ctx context.Context, request Request) (<-chan StreamEvent, error)
}

type CallContext struct {
	UserID string
	RunID  string
}

type Generator interface {
	ProviderName() string
	Generate(ctx context.Context, call CallContext, request Request) (Response, error)
	Stream(ctx context.Context, call CallContext, request Request) (<-chan StreamEvent, error)
}

type UsageRecorder interface {
	RecordUsage(ctx context.Context, record domain.UsageRecord) error
}
