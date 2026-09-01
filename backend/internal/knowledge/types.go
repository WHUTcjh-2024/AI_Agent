package knowledge

import (
	"context"
	"time"
)

type Request struct {
	SchoolID string
	Query    string
	RunID    string
	TopN     int
}

type ProviderRequest struct {
	Query            string
	KnowledgeBaseIDs []string
	RequestID        string
	Limit            int
}

type Evidence struct {
	ChunkID     string
	KnowledgeID string
	SourceID    string
	Title       string
	Filename    string
	Content     string
	SourceURL   string
	Publisher   string
	PublishedAt *time.Time
	Score       float64
	Metadata    map[string]any
}

type Stats struct {
	Provider   string `json:"provider"`
	Configured bool   `json:"configured"`
	Hits       int    `json:"hits"`
}

type Response struct {
	Evidence []Evidence
	Stats    Stats
}

type Searcher interface {
	Search(ctx context.Context, request Request) (Response, error)
}

type Provider interface {
	Name() string
	Search(ctx context.Context, request ProviderRequest) ([]Evidence, error)
}

// ScopeResolver keeps provider code independent from AskU's school registry.
// Every school selects its own knowledge-base identifiers through configuration.
type ScopeResolver interface {
	OfficialKnowledgeBaseID(schoolID string) (string, error)
	SchoolName(schoolID string) (string, error)
	AllowedDomains(schoolID string) ([]string, error)
}
