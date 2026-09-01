package websearch

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type testScopes struct{}

func (testScopes) AllowedDomains(string) ([]string, error) { return []string{"whut.edu.cn"}, nil }

type countingProvider struct{ calls int }

func (p *countingProvider) Name() string { return "counting" }
func (p *countingProvider) Search(context.Context, ProviderRequest) ([]SearchResult, error) {
	p.calls++
	return []SearchResult{
		{Title: "A", URL: "https://jwc.whut.edu.cn/a"},
		{Title: "blocked", URL: "https://whut.edu.cn.evil.example/b"},
		{Title: "B", URL: "https://lib.whut.edu.cn/b"},
	}, nil
}

type countingFetcher struct{ calls int }

func (f *countingFetcher) Fetch(_ context.Context, rawURL string, scope Scope) (Page, error) {
	f.calls++
	if !IsAllowedURL(rawURL, scope.AllowedDomains) {
		return Page{}, ErrDisallowedURL
	}
	return Page{URL: rawURL, ContentType: "text/html", Body: "<p>官网搜索联调相关内容</p>", FetchedAt: time.Now().UTC()}, nil
}

type memoryCache struct {
	mu     sync.Mutex
	values map[string][]byte
}

func newMemoryCache() *memoryCache { return &memoryCache{values: map[string][]byte{}} }
func (c *memoryCache) GetJSON(_ context.Context, key string, target any) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.values[key]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(value, target)
}
func (c *memoryCache) SetJSON(_ context.Context, key string, value any, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.values[key] = encoded
	return nil
}

func TestGatewayFiltersAllowlistFetchesTopNAndCachesEachStage(t *testing.T) {
	provider := &countingProvider{}
	fetcher := &countingFetcher{}
	gateway, err := NewGateway(provider, fetcher, NewHTMLExtractor(2, 300), newMemoryCache(), testScopes{}, CachePolicy{time.Minute, time.Minute, time.Minute})
	if err != nil {
		t.Fatal(err)
	}

	first, err := gateway.Gather(context.Background(), Request{SchoolID: "whut", Query: "官网搜索", TopN: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Evidence) != 2 {
		t.Fatalf("got %d evidence items, want 2", len(first.Evidence))
	}
	if provider.calls != 1 || fetcher.calls != 2 {
		t.Fatalf("unexpected calls: provider=%d fetcher=%d", provider.calls, fetcher.calls)
	}
	if first.Stats.PagesFetched != 2 || first.Stats.SearchCacheHit {
		t.Fatalf("unexpected first stats: %+v", first.Stats)
	}

	second, err := gateway.Gather(context.Background(), Request{SchoolID: "whut", Query: "官网搜索", TopN: 2})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || fetcher.calls != 2 {
		t.Fatalf("cache did not isolate providers: provider=%d fetcher=%d", provider.calls, fetcher.calls)
	}
	if !second.Stats.SearchCacheHit || second.Stats.PageCacheHits != 2 || second.Stats.ExtractCacheHits != 2 {
		t.Fatalf("unexpected second stats: %+v", second.Stats)
	}
	for _, evidence := range second.Evidence {
		if !evidence.Official || !IsAllowedURL(evidence.URL, []string{"whut.edu.cn"}) {
			t.Fatalf("non-official evidence escaped gateway: %+v", evidence)
		}
	}
}

func TestGatewayRejectsInvalidDependencies(t *testing.T) {
	_, err := NewGateway(nil, nil, nil, nil, nil, CachePolicy{})
	if err == nil {
		t.Fatal("expected dependency validation error")
	}
	if _, err = (&Gateway{}).Gather(context.Background(), Request{}); err != ErrInvalidQuery {
		t.Fatalf("got %v", err)
	}
}

func TestSourceIdentityIsScopedBySchool(t *testing.T) {
	whut := sourceID("whut", "https://shared.example/notice")
	hzau := sourceID("hzau", "https://shared.example/notice")
	if whut == hzau {
		t.Fatalf("source IDs must not collide across schools: %s", whut)
	}
}
