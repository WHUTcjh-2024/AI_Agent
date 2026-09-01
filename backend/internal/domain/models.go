package domain

import (
	"encoding/json"
	"time"
)

type User struct {
	ID         string `json:"id"`
	Nickname   string `json:"nickname"`
	AvatarURL  string `json:"avatarUrl,omitempty"`
	SchoolID   string `json:"schoolId"`
	SchoolName string `json:"schoolName"`
}

type Message struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	SourceIDs []string  `json:"sourceIds,omitempty"`
	Status    string    `json:"status"`
}

type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Messages  []Message `json:"messages"`
}

type Source struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Publisher   string    `json:"publisher"`
	PublishedAt time.Time `json:"publishedAt"`
	Audience    string    `json:"audience"`
	Summary     string    `json:"summary"`
	URL         string    `json:"url"`
	Official    bool      `json:"official"`
}

type AgentRun struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type RunEvent struct {
	Sequence  int64           `json:"sequence"`
	RunID     string          `json:"runId"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"createdAt"`
}

type Feedback struct {
	ID        string    `json:"id"`
	MessageID string    `json:"messageId"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"createdAt"`
}

type UsageRecord struct {
	ID                    string    `json:"id"`
	UserID                string    `json:"userId"`
	RunID                 string    `json:"runId,omitempty"`
	Provider              string    `json:"provider"`
	Model                 string    `json:"model"`
	InputTokens           int       `json:"inputTokens"`
	OutputTokens          int       `json:"outputTokens"`
	EstimatedCostMicroRMB int64     `json:"estimatedCostMicroRmb"`
	LatencyMS             int64     `json:"latencyMs"`
	Status                string    `json:"status"`
	ErrorCode             string    `json:"errorCode,omitempty"`
	TokensEstimated       bool      `json:"tokensEstimated"`
	CreatedAt             time.Time `json:"createdAt"`
}

type TokenPair struct {
	AccessToken      string    `json:"accessToken"`
	AccessExpiresAt  time.Time `json:"accessExpiresAt"`
	RefreshToken     string    `json:"refreshToken"`
	RefreshExpiresAt time.Time `json:"refreshExpiresAt"`
	User             User      `json:"user"`
}
