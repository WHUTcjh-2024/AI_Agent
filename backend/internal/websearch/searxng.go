package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type SearXNGProvider struct {
	baseURL *url.URL
	apiKey  string
	client  *http.Client
}

func NewSearXNGProvider(baseURL, apiKey string, client *http.Client) (*SearXNGProvider, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || !parsed.IsAbs() || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("ASKU_WEB_SEARCH_BASE_URL must be an absolute HTTP(S) URL")
	}
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return &SearXNGProvider{baseURL: parsed, apiKey: strings.TrimSpace(apiKey), client: client}, nil
}

func (p *SearXNGProvider) Name() string { return "searxng" }

func (p *SearXNGProvider) Search(ctx context.Context, searchRequest ProviderRequest) ([]SearchResult, error) {
	endpoint := *p.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/search"
	values := endpoint.Query()
	values.Set("q", scopedQuery(searchRequest.Query, searchRequest.AllowedDomains))
	values.Set("format", "json")
	if searchRequest.Limit > 0 {
		values.Set("limit", strconv.Itoa(searchRequest.Limit))
	}
	endpoint.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create search request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if p.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call search provider: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("search provider returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		Results []struct {
			Title         string `json:"title"`
			URL           string `json:"url"`
			Content       string `json:"content"`
			PublishedDate string `json:"publishedDate"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	results := make([]SearchResult, 0, len(payload.Results))
	for _, item := range payload.Results {
		var publishedAt *time.Time
		if parsed := parsePublishedAt(item.PublishedDate); !parsed.IsZero() {
			publishedAt = &parsed
		}
		results = append(results, SearchResult{Title: item.Title, URL: item.URL, Snippet: item.Content, Publisher: hostLabel(item.URL), PublishedAt: publishedAt})
		if searchRequest.Limit > 0 && len(results) >= searchRequest.Limit {
			break
		}
	}
	return results, nil
}

func scopedQuery(query string, domains []string) string {
	clauses := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" || strings.ContainsAny(domain, " /:()") {
			continue
		}
		clauses = append(clauses, "site:"+domain)
	}
	if len(clauses) == 0 {
		return strings.TrimSpace(query)
	}
	return strings.TrimSpace(query) + " (" + strings.Join(clauses, " OR ") + ")"
}

func parsePublishedAt(value string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
