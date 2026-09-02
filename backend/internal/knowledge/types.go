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
	ChunkID           string
	KnowledgeID       string
	AskUDocumentID    string
	SourceID          string
	Title             string
	Filename          string
	Content           string
	SourceURL         string
	OfficialURL       string
	AttachmentURL     string
	ParentPageURL     string
	Publisher         string
	Department        string
	SourceType        string
	DocumentType      string
	Authority         string
	Freshness         string
	KnowledgeBundleID string
	PublishedAt       *time.Time
	Attachments       []Attachment
	Score             float64
	Metadata          map[string]any
}

type Attachment struct {
	ID            string
	Name          string
	URL           string
	DocumentType  string
	ParentPageURL string
}

type DocumentMetadata struct {
	AskUDocumentID    string
	Title             string
	SourceName        string
	Department        string
	PublishedAt       *time.Time
	SourceType        string
	DocumentType      string
	OfficialURL       string
	CanonicalURL      string
	AttachmentURL     string
	ParentPageURL     string
	Authority         string
	Freshness         string
	KnowledgeBundleID string
	Attachments       []Attachment
}

// Catalog resolves provider-owned retrieval IDs to crawler-owned public
// metadata. It deliberately excludes local_file_path from the contract.
type Catalog interface {
	ResolveEvidence(ctx context.Context, schoolID, knowledgeID string) (DocumentMetadata, bool, error)
}

type Stats struct {
	Provider      string `json:"provider"`
	Configured    bool   `json:"configured"`
	Hits          int    `json:"hits"`
	QueryCacheHit bool   `json:"queryCacheHit"`
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
	KnowledgeVersion(schoolID string) (string, error)
	AllowedDomains(schoolID string) ([]string, error)
}

// JSONCache keeps Redis details outside the knowledge domain.
type JSONCache interface {
	GetJSON(ctx context.Context, key string, target any) (bool, error)
	SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
}

type CachePolicy struct {
	QueryTTL time.Duration
}
