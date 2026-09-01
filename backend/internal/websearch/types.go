package websearch

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidQuery     = errors.New("web search query is empty")
	ErrDisallowedURL    = errors.New("URL is outside the school allowlist")
	ErrUnsupportedMedia = errors.New("page content type is not supported")
)

type Request struct {
	SchoolID string
	Query    string
	TopN     int
}

type Scope struct {
	SchoolID       string
	AllowedDomains []string
}

type SearchResult struct {
	Title       string     `json:"title"`
	URL         string     `json:"url"`
	Snippet     string     `json:"snippet"`
	Publisher   string     `json:"publisher"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
}

type ProviderRequest struct {
	Query          string
	AllowedDomains []string
	Limit          int
}

type Page struct {
	URL         string    `json:"url"`
	ContentType string    `json:"contentType"`
	Body        string    `json:"body"`
	FetchedAt   time.Time `json:"fetchedAt"`
}

type Section struct {
	Text  string `json:"text"`
	Score int    `json:"score"`
}

type Evidence struct {
	ID          string
	Title       string
	URL         string
	Publisher   string
	PublishedAt time.Time
	Excerpt     string
	Official    bool
}

type Stats struct {
	SearchCacheHit   bool `json:"searchCacheHit"`
	PageCacheHits    int  `json:"pageCacheHits"`
	ExtractCacheHits int  `json:"extractCacheHits"`
	PagesFetched     int  `json:"pagesFetched"`
	PagesFailed      int  `json:"pagesFailed"`
}

type Response struct {
	Evidence []Evidence
	Stats    Stats
}

type Searcher interface {
	Gather(ctx context.Context, request Request) (Response, error)
}

type Provider interface {
	Name() string
	Search(ctx context.Context, request ProviderRequest) ([]SearchResult, error)
}

type Fetcher interface {
	Fetch(ctx context.Context, rawURL string, scope Scope) (Page, error)
}

type Extractor interface {
	ExtractRelevantSections(ctx context.Context, query string, page Page) ([]Section, error)
}

type ScopeResolver interface {
	AllowedDomains(schoolID string) ([]string, error)
}

// JSONCache keeps Redis details outside the search domain.
type JSONCache interface {
	GetJSON(ctx context.Context, key string, target any) (bool, error)
	SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
}

type CachePolicy struct {
	SearchTTL  time.Duration
	PageTTL    time.Duration
	ExtractTTL time.Duration
}
