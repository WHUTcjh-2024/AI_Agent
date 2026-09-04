package school_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"asku/backend/internal/agent"
	"asku/backend/internal/domain"
	"asku/backend/internal/knowledge"
	"asku/backend/internal/school"
	"asku/backend/internal/websearch"
	"gopkg.in/yaml.v3"
)

type memoryCache map[string][]byte

func (c memoryCache) GetJSON(_ context.Context, key string, target any) (bool, error) {
	data, ok := c[key]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(data, target)
}
func (c memoryCache) SetJSON(_ context.Context, key string, value any, _ time.Duration) error {
	data, err := json.Marshal(value)
	c[key] = data
	return err
}

type knowledgeProvider struct{ requests []knowledge.ProviderRequest }

func (*knowledgeProvider) Name() string { return "portable-fixture" }
func (p *knowledgeProvider) Search(_ context.Context, r knowledge.ProviderRequest) ([]knowledge.Evidence, error) {
	p.requests = append(p.requests, r)
	return []knowledge.Evidence{
		{KnowledgeID: "test-document", ChunkID: "test-chunk", Content: "Test rule", SourceURL: "https://example.edu.cn/rule"},
		{KnowledgeID: "other-document", ChunkID: "other-chunk", Content: "Other rule", SourceURL: "https://jwc.whut.edu.cn/rule"},
	}, nil
}

type webProvider struct{ requests []websearch.ProviderRequest }

func (*webProvider) Name() string { return "portable-fixture" }
func (p *webProvider) Search(_ context.Context, r websearch.ProviderRequest) ([]websearch.SearchResult, error) {
	p.requests = append(p.requests, r)
	return []websearch.SearchResult{{URL: "https://example.edu.cn/rule", Title: "Test rule"}, {URL: "https://jwc.whut.edu.cn/rule", Title: "Other rule"}}, nil
}

type pageFetcher struct{ scopes []websearch.Scope }

func (p *pageFetcher) Fetch(_ context.Context, url string, scope websearch.Scope) (websearch.Page, error) {
	p.scopes = append(p.scopes, scope)
	return websearch.Page{URL: url, ContentType: "text/html", Body: "<p>Official rule evidence.</p>"}, nil
}

// Two independent single-school deployments share storage here to make scope
// leaks observable. Both use the actual loader, gateways and versioned cache.
func TestSingleSchoolPortability(t *testing.T) {
	ctx := context.Background()
	cache := memoryCache{}
	kp, wp, fetcher := &knowledgeProvider{}, &webProvider{}, &pageFetcher{}
	var previousSource string
	for _, name := range []string{"whut", "testu"} {
		t.Run(name, func(t *testing.T) {
			path := "../../../evals/fixtures/testu.yaml"
			if name == "whut" {
				data, err := os.ReadFile("../../../config/schools/whut.yaml")
				if err != nil {
					t.Fatal(err)
				}
				var raw map[string]any
				if err = yaml.Unmarshal(data, &raw); err != nil {
					t.Fatal(err)
				}
				raw["official_knowledge_base_id"] = "kb-whut"
				raw["knowledge_version"] = "test-v1"
				data, err = yaml.Marshal(raw)
				if err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(t.TempDir(), "school.yaml")
				if err = os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			registry, err := school.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			current := registry.Current()
			kg, err := knowledge.NewGateway(kp, registry, cache, knowledge.CachePolicy{QueryTTL: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			wg, err := websearch.NewGateway(wp, fetcher, websearch.NewHTMLExtractor(3, 1200), cache, registry, websearch.CachePolicy{SearchTTL: time.Minute, PageTTL: time.Minute, ExtractTTL: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			ac, err := agent.NewVersionedAnswerCache(cache, registry, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if _, hit, err := ac.Lookup(ctx, name, "same question"); err != nil || hit {
				t.Fatalf("cross-school answer cache hit=%v err=%v", hit, err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				kr, err := kg.Search(ctx, knowledge.Request{SchoolID: name, Query: "same question", TopN: 3})
				if err != nil {
					t.Fatal(err)
				}
				wr, err := wg.Gather(ctx, websearch.Request{SchoolID: name, Query: "same question", TopN: 3})
				if err != nil {
					t.Fatal(err)
				}
				if kr.Stats.QueryCacheHit != (attempt == 1) || wr.Stats.SearchCacheHit != (attempt == 1) || len(kr.Evidence) != 1 || len(wr.Evidence) != 1 {
					t.Fatalf("cache or source scope failed: %+v %+v", kr, wr)
				}
				if name == "testu" && (strings.Contains(kr.Evidence[0].SourceURL, "whut") || strings.Contains(wr.Evidence[0].URL, "whut")) {
					t.Fatal("foreign evidence leaked")
				}
				if attempt == 0 {
					if kr.Evidence[0].SourceID == previousSource {
						t.Fatal("source ID scope collision")
					}
					previousSource = kr.Evidence[0].SourceID
					if !reflect.DeepEqual(kp.requests[len(kp.requests)-1].KnowledgeBaseIDs, []string{current.OfficialKnowledgeBaseID}) {
						t.Fatal("wrong KB")
					}
					if !reflect.DeepEqual(wp.requests[len(wp.requests)-1].AllowedDomains, current.AllowedDomains) {
						t.Fatal("wrong web domains")
					}
				}
			}
			answer := agent.CachedAnswer{Answer: "Verified", Sources: []domain.Source{{ID: name, Official: true}}, Citations: []domain.Citation{{CitationID: "citation", Index: 1, SourceID: name, EvidenceText: "Evidence", OfficialURL: "https://" + current.AllowedDomains[0] + "/rule"}}}
			if err := ac.Store(ctx, name, "same question", answer); err != nil {
				t.Fatal(err)
			}
			if _, hit, err := ac.Lookup(ctx, name, "same question"); err != nil || !hit {
				t.Fatal("school cache roundtrip failed")
			}
			if _, err := kg.Search(ctx, knowledge.Request{SchoolID: "unknown", Query: "same question", TopN: 3}); err == nil {
				t.Fatal("unknown school accepted")
			}
			if _, err := wg.Gather(ctx, websearch.Request{SchoolID: "unknown", Query: "same question"}); err == nil {
				t.Fatal("unknown school accepted")
			}
		})
	}
	if len(kp.requests) != 2 || len(wp.requests) != 2 || len(fetcher.scopes) != 2 {
		t.Fatal("cache missed or leaked across deployments")
	}
}
