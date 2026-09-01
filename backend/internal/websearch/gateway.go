package websearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	defaultTopN = 3
	maxTopN     = 5
)

type Gateway struct {
	provider  Provider
	fetcher   Fetcher
	extractor Extractor
	cache     JSONCache
	scopes    ScopeResolver
	policy    CachePolicy
}

func NewGateway(provider Provider, fetcher Fetcher, extractor Extractor, cache JSONCache, scopes ScopeResolver, policy CachePolicy) (*Gateway, error) {
	if provider == nil || fetcher == nil || extractor == nil || scopes == nil {
		return nil, fmt.Errorf("web search gateway dependencies must not be nil")
	}
	if policy.SearchTTL <= 0 || policy.PageTTL <= 0 || policy.ExtractTTL <= 0 {
		return nil, fmt.Errorf("web search cache TTLs must be positive")
	}
	return &Gateway{provider: provider, fetcher: fetcher, extractor: extractor, cache: cache, scopes: scopes, policy: policy}, nil
}

func (g *Gateway) ProviderName() string { return g.provider.Name() }

func (g *Gateway) Gather(ctx context.Context, request Request) (Response, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return Response{}, ErrInvalidQuery
	}
	domains, err := g.scopes.AllowedDomains(request.SchoolID)
	if err != nil {
		return Response{}, fmt.Errorf("resolve school search scope: %w", err)
	}
	scope := Scope{SchoolID: request.SchoolID, AllowedDomains: domains}
	topN := request.TopN
	if topN <= 0 {
		topN = defaultTopN
	}
	if topN > maxTopN {
		topN = maxTopN
	}

	response := Response{Evidence: []Evidence{}}
	results, hit := g.cachedSearch(ctx, scope, query)
	response.Stats.SearchCacheHit = hit
	if !hit {
		results, err = g.provider.Search(ctx, ProviderRequest{Query: query, AllowedDomains: scope.AllowedDomains, Limit: maxTopN * 2})
		if err != nil {
			return Response{}, fmt.Errorf("search with %s: %w", g.provider.Name(), err)
		}
		results = filterAllowedResults(results, scope.AllowedDomains)
		g.storeCache(ctx, searchCacheKey(scope.SchoolID, query), results, g.policy.SearchTTL)
	} else {
		results = filterAllowedResults(results, scope.AllowedDomains)
	}
	if len(results) > topN {
		results = results[:topN]
	}

	for _, result := range results {
		page, pageHit, fetchErr := g.page(ctx, result.URL, scope)
		if fetchErr != nil {
			response.Stats.PagesFailed++
			slog.Warn("skip web search page", "provider", g.provider.Name(), "url", result.URL, "error", fetchErr)
			continue
		}
		if pageHit {
			response.Stats.PageCacheHits++
		} else {
			response.Stats.PagesFetched++
		}

		sections, extractHit, extractErr := g.sections(ctx, query, page)
		if extractErr != nil {
			response.Stats.PagesFailed++
			slog.Warn("skip web search extraction", "url", result.URL, "error", extractErr)
			continue
		}
		if extractHit {
			response.Stats.ExtractCacheHits++
		}
		excerpt := strings.TrimSpace(result.Snippet)
		if len(sections) > 0 {
			parts := make([]string, 0, len(sections))
			for _, section := range sections {
				parts = append(parts, section.Text)
			}
			excerpt = strings.Join(parts, "\n")
		}
		// Never present retrieval time as the document publication time.
		publishedAt := time.Time{}
		if result.PublishedAt != nil {
			publishedAt = result.PublishedAt.UTC()
		}
		response.Evidence = append(response.Evidence, Evidence{
			ID: sourceID(scope.SchoolID, result.URL), Title: fallback(result.Title, result.URL), URL: result.URL,
			Publisher: fallback(result.Publisher, hostLabel(result.URL)), PublishedAt: publishedAt,
			Excerpt: excerpt, Official: true,
		})
	}
	return response, nil
}

func (g *Gateway) cachedSearch(ctx context.Context, scope Scope, query string) ([]SearchResult, bool) {
	if g.cache == nil {
		return nil, false
	}
	var results []SearchResult
	hit, err := g.cache.GetJSON(ctx, searchCacheKey(scope.SchoolID, query), &results)
	if err != nil {
		slog.Warn("read web search cache", "kind", "search", "error", err)
		return nil, false
	}
	return results, hit
}

func (g *Gateway) page(ctx context.Context, rawURL string, scope Scope) (Page, bool, error) {
	key := pageCacheKey(rawURL)
	if g.cache != nil {
		var page Page
		if hit, err := g.cache.GetJSON(ctx, key, &page); err == nil && hit {
			return page, true, nil
		} else if err != nil {
			slog.Warn("read web search cache", "kind", "page", "error", err)
		}
	}
	page, err := g.fetcher.Fetch(ctx, rawURL, scope)
	if err != nil {
		return Page{}, false, err
	}
	g.storeCache(ctx, key, page, g.policy.PageTTL)
	return page, false, nil
}

func (g *Gateway) sections(ctx context.Context, query string, page Page) ([]Section, bool, error) {
	key := extractCacheKey(query, page.URL)
	if g.cache != nil {
		var sections []Section
		if hit, err := g.cache.GetJSON(ctx, key, &sections); err == nil && hit {
			return sections, true, nil
		} else if err != nil {
			slog.Warn("read web search cache", "kind", "extract", "error", err)
		}
	}
	sections, err := g.extractor.ExtractRelevantSections(ctx, query, page)
	if err != nil {
		return nil, false, err
	}
	g.storeCache(ctx, key, sections, g.policy.ExtractTTL)
	return sections, false, nil
}

func (g *Gateway) storeCache(ctx context.Context, key string, value any, ttl time.Duration) {
	if g.cache == nil {
		return
	}
	if err := g.cache.SetJSON(ctx, key, value, ttl); err != nil {
		slog.Warn("write web search cache", "key", key, "error", err)
	}
}

func filterAllowedResults(results []SearchResult, domains []string) []SearchResult {
	filtered := make([]SearchResult, 0, len(results))
	seen := make(map[string]struct{}, len(results))
	for _, result := range results {
		result.URL = strings.TrimSpace(result.URL)
		if !IsAllowedURL(result.URL, domains) {
			continue
		}
		if _, exists := seen[result.URL]; exists {
			continue
		}
		seen[result.URL] = struct{}{}
		filtered = append(filtered, result)
	}
	return filtered
}

func searchCacheKey(schoolID, query string) string {
	return "search:" + schoolID + ":" + digest(strings.ToLower(strings.TrimSpace(query)))
}
func pageCacheKey(rawURL string) string { return "page:" + digest(rawURL) }
func extractCacheKey(query, rawURL string) string {
	return "extract:" + digest(strings.ToLower(strings.TrimSpace(query))+"\x00"+rawURL)
}
func sourceID(schoolID, rawURL string) string {
	return "src_web_" + digest(strings.TrimSpace(schoolID) + "\x00" + strings.TrimSpace(rawURL))[:24]
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func fallback(value, alternative string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return alternative
}
