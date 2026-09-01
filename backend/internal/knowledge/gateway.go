package knowledge

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"
)

type Gateway struct {
	provider Provider
	scopes   ScopeResolver
}

func NewGateway(provider Provider, scopes ScopeResolver) (*Gateway, error) {
	if provider == nil {
		return nil, fmt.Errorf("knowledge provider must not be nil")
	}
	if scopes == nil {
		return nil, fmt.Errorf("knowledge scope resolver must not be nil")
	}
	return &Gateway{provider: provider, scopes: scopes}, nil
}

func (g *Gateway) Search(ctx context.Context, request Request) (Response, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return Response{}, fmt.Errorf("knowledge query must not be empty")
	}
	if request.TopN < 1 || request.TopN > 10 {
		return Response{}, fmt.Errorf("knowledge Top-N must be between 1 and 10")
	}
	knowledgeBaseID, err := g.scopes.OfficialKnowledgeBaseID(request.SchoolID)
	if err != nil {
		return Response{}, err
	}
	if strings.TrimSpace(knowledgeBaseID) == "" {
		return Response{Evidence: []Evidence{}, Stats: Stats{Provider: g.provider.Name(), Configured: false}}, nil
	}
	schoolName, err := g.scopes.SchoolName(request.SchoolID)
	if err != nil {
		return Response{}, err
	}
	allowedDomains, err := g.scopes.AllowedDomains(request.SchoolID)
	if err != nil {
		return Response{}, err
	}
	evidence, err := g.provider.Search(ctx, ProviderRequest{
		Query: query, KnowledgeBaseIDs: []string{knowledgeBaseID}, RequestID: request.RunID, Limit: request.TopN,
	})
	if err != nil {
		return Response{}, err
	}
	normalized := make([]Evidence, 0, len(evidence))
	for _, item := range evidence {
		item.KnowledgeID = strings.TrimSpace(item.KnowledgeID)
		item.Content = strings.TrimSpace(item.Content)
		if item.KnowledgeID == "" || item.Content == "" {
			continue
		}
		item.SourceID = sourceID(request.SchoolID, item.KnowledgeID)
		item.SourceURL = allowedSourceURL(item.SourceURL, allowedDomains)
		item.Title = truncateText(strings.TrimSpace(item.Title), 300)
		item.Filename = truncateText(strings.TrimSpace(item.Filename), 300)
		item.Publisher = strings.TrimSpace(item.Publisher)
		if item.Publisher == "" {
			item.Publisher = schoolName
		}
		item.Publisher = truncateText(item.Publisher, 160)
		normalized = append(normalized, item)
		if len(normalized) == request.TopN {
			break
		}
	}
	return Response{
		Evidence: normalized,
		Stats:    Stats{Provider: g.provider.Name(), Configured: true, Hits: len(normalized)},
	}, nil
}

func allowedSourceURL(rawURL string, allowedDomains []string) string {
	rawURL = strings.TrimSpace(rawURL)
	if len(rawURL) > 2048 {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return ""
	}
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	for _, domain := range allowedDomains {
		domain = strings.ToLower(strings.Trim(strings.TrimSpace(domain), "."))
		if domain != "" && (hostname == domain || strings.HasSuffix(hostname, "."+domain)) {
			return parsed.String()
		}
	}
	return ""
}

func truncateText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func sourceID(schoolID, knowledgeID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(schoolID) + "\x00" + strings.TrimSpace(knowledgeID)))
	return fmt.Sprintf("src_kb_%x", digest[:12])
}

// DisabledSearcher is the explicit pre-WeKnora runtime state. It never emits
// fixture evidence, so the product can safely return an honest no-source answer.
type DisabledSearcher struct{}

func NewDisabledSearcher() DisabledSearcher { return DisabledSearcher{} }

func (DisabledSearcher) Search(context.Context, Request) (Response, error) {
	return Response{Evidence: []Evidence{}, Stats: Stats{Provider: "disabled", Configured: false}}, nil
}
