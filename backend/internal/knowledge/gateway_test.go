package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

type scopeStub struct {
	knowledgeBases map[string]string
	names          map[string]string
	domains        map[string][]string
}

func (s scopeStub) AllowedDomains(schoolID string) ([]string, error) {
	value, ok := s.domains[schoolID]
	if !ok {
		return nil, fmt.Errorf("unknown school")
	}
	return value, nil
}

func (s scopeStub) OfficialKnowledgeBaseID(schoolID string) (string, error) {
	value, ok := s.knowledgeBases[schoolID]
	if !ok {
		return "", fmt.Errorf("unknown school")
	}
	return value, nil
}

func (s scopeStub) SchoolName(schoolID string) (string, error) {
	value, ok := s.names[schoolID]
	if !ok {
		return "", fmt.Errorf("unknown school")
	}
	return value, nil
}

func (s scopeStub) KnowledgeVersion(schoolID string) (string, error) {
	if _, ok := s.knowledgeBases[schoolID]; !ok {
		return "", fmt.Errorf("unknown school")
	}
	return "v1", nil
}

type providerStub struct {
	calls   int
	request ProviderRequest
}

func (p *providerStub) Name() string { return "stub" }
func (p *providerStub) Search(_ context.Context, request ProviderRequest) ([]Evidence, error) {
	p.calls++
	p.request = request
	return []Evidence{{ChunkID: "chunk", KnowledgeID: "knowledge", Content: "官方资料"}}, nil
}

func TestGatewayResolvesSchoolKnowledgeScope(t *testing.T) {
	provider := &providerStub{}
	gateway, err := NewGateway(provider, scopeStub{
		knowledgeBases: map[string]string{"whut": "kb-whut"}, names: map[string]string{"whut": "武汉理工大学"},
		domains: map[string][]string{"whut": {"whut.edu.cn"}},
	}, nil, CachePolicy{QueryTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	response, err := gateway.Search(context.Background(), Request{SchoolID: "whut", Query: "奖学金", RunID: "run-1", TopN: 4})
	if err != nil {
		t.Fatal(err)
	}
	if provider.request.KnowledgeBaseIDs[0] != "kb-whut" || provider.request.RequestID != "run-1" {
		t.Fatalf("provider did not receive school scope: %#v", provider.request)
	}
	if len(response.Evidence) != 1 || response.Evidence[0].Publisher != "武汉理工大学" || response.Evidence[0].SourceID == "" {
		t.Fatalf("evidence was not normalized: %#v", response)
	}
}

func TestGatewayDoesNotCallProviderWithoutConfiguredKnowledgeBase(t *testing.T) {
	provider := &providerStub{}
	gateway, err := NewGateway(provider, scopeStub{
		knowledgeBases: map[string]string{"whut": ""}, names: map[string]string{"whut": "武汉理工大学"},
		domains: map[string][]string{"whut": {"whut.edu.cn"}},
	}, nil, CachePolicy{QueryTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	response, err := gateway.Search(context.Background(), Request{SchoolID: "whut", Query: "选课", TopN: 4})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 0 || response.Stats.Configured || len(response.Evidence) != 0 {
		t.Fatalf("unconfigured school must stay explicit and empty: %#v", response)
	}
}

func TestGatewayDropsKnowledgeSourceURLOutsideSchoolAllowlist(t *testing.T) {
	providerEvidence := "https://evil.example/notice"
	providerWithURL := &providerWithEvidence{evidence: []Evidence{{
		ChunkID: "chunk", KnowledgeID: "knowledge", Content: "官方资料", SourceURL: providerEvidence,
	}}}
	gateway, err := NewGateway(providerWithURL, scopeStub{
		knowledgeBases: map[string]string{"whut": "kb-whut"}, names: map[string]string{"whut": "武汉理工大学"},
		domains: map[string][]string{"whut": {"whut.edu.cn"}},
	}, nil, CachePolicy{QueryTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	response, err := gateway.Search(context.Background(), Request{SchoolID: "whut", Query: "通知", TopN: 4})
	if err != nil {
		t.Fatal(err)
	}
	if response.Evidence[0].SourceURL != "" {
		t.Fatalf("external URL must not be exposed as an official source: %s", response.Evidence[0].SourceURL)
	}
}

func TestGatewayKeepsKnowledgeSourceURLWithinSchoolAllowlist(t *testing.T) {
	const sourceURL = "https://jwc.whut.edu.cn/tzgg/notice.htm"
	providerWithURL := &providerWithEvidence{evidence: []Evidence{{
		ChunkID: "chunk", KnowledgeID: "knowledge", Content: "官方资料", SourceURL: sourceURL,
	}}}
	gateway, err := NewGateway(providerWithURL, scopeStub{
		knowledgeBases: map[string]string{"whut": "kb-whut"}, names: map[string]string{"whut": "武汉理工大学"},
		domains: map[string][]string{"whut": {"whut.edu.cn"}},
	}, nil, CachePolicy{QueryTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	response, err := gateway.Search(context.Background(), Request{SchoolID: "whut", Query: "通知", TopN: 4})
	if err != nil {
		t.Fatal(err)
	}
	if response.Evidence[0].SourceURL != sourceURL {
		t.Fatalf("official school URL was unexpectedly removed: %s", response.Evidence[0].SourceURL)
	}
}

func TestGatewayRejectsMalformedProviderEvidence(t *testing.T) {
	providerEvidence := &providerWithEvidence{evidence: []Evidence{
		{ChunkID: "missing-id", Content: "正文"},
		{ChunkID: "missing-content", KnowledgeID: "knowledge"},
		{ChunkID: "valid", KnowledgeID: "valid-knowledge", Content: "有效正文"},
	}}
	gateway, err := NewGateway(providerEvidence, scopeStub{
		knowledgeBases: map[string]string{"whut": "kb-whut"}, names: map[string]string{"whut": "武汉理工大学"},
		domains: map[string][]string{"whut": {"whut.edu.cn"}},
	}, nil, CachePolicy{QueryTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	response, err := gateway.Search(context.Background(), Request{SchoolID: "whut", Query: "通知", TopN: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Evidence) != 1 || response.Stats.Hits != 1 || response.Evidence[0].ChunkID != "valid" {
		t.Fatalf("gateway accepted malformed provider evidence: %#v", response)
	}
}

type providerWithEvidence struct{ evidence []Evidence }

func (p *providerWithEvidence) Name() string { return "stub" }
func (p *providerWithEvidence) Search(context.Context, ProviderRequest) ([]Evidence, error) {
	return p.evidence, nil
}

type catalogStub struct {
	metadata DocumentMetadata
	found    bool
}

func (c catalogStub) ResolveEvidence(context.Context, string, string) (DocumentMetadata, bool, error) {
	return c.metadata, c.found, nil
}

func TestGatewayRequiresCrawlerMappingWhenCatalogIsConfigured(t *testing.T) {
	provider := &providerWithEvidence{evidence: []Evidence{{ChunkID: "chunk", KnowledgeID: "knowledge", Content: "官方资料", SourceURL: "https://jwc.whut.edu.cn/provider-only"}}}
	gateway, err := NewGateway(provider, scopeStub{
		knowledgeBases: map[string]string{"whut": "kb-whut"}, names: map[string]string{"whut": "武汉理工大学"},
		domains: map[string][]string{"whut": {"whut.edu.cn"}},
	}, nil, CachePolicy{QueryTTL: time.Minute}, catalogStub{found: false})
	if err != nil {
		t.Fatal(err)
	}
	response, err := gateway.Search(context.Background(), Request{SchoolID: "whut", Query: "通知", TopN: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Evidence) != 0 {
		t.Fatalf("unmapped WeKnora evidence must not ground an answer: %#v", response.Evidence)
	}
}

func TestGatewayUsesCrawlerMetadataAndAttachmentPriority(t *testing.T) {
	publishedAt := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
	provider := &providerWithEvidence{evidence: []Evidence{{ChunkID: "chunk", KnowledgeID: "knowledge", Content: "5 月 20 日截止"}}}
	gateway, err := NewGateway(provider, scopeStub{
		knowledgeBases: map[string]string{"whut": "kb-whut"}, names: map[string]string{"whut": "武汉理工大学"},
		domains: map[string][]string{"whut": {"whut.edu.cn"}},
	}, nil, CachePolicy{QueryTTL: time.Minute}, catalogStub{found: true, metadata: DocumentMetadata{
		AskUDocumentID: "doc-1", Title: "转专业通知", SourceName: "武汉理工大学本科生院", Department: "本科生院",
		PublishedAt: &publishedAt, OfficialURL: "https://jwc.whut.edu.cn/notice.htm",
		AttachmentURL: "https://jwc.whut.edu.cn/files/plan.pdf", ParentPageURL: "https://jwc.whut.edu.cn/notice.htm",
		Authority: "OFFICIAL_DEPARTMENT", DocumentType: "PDF",
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := gateway.Search(context.Background(), Request{SchoolID: "whut", Query: "通知", TopN: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Evidence) != 1 || response.Evidence[0].AskUDocumentID != "doc-1" || response.Evidence[0].SourceURL != "https://jwc.whut.edu.cn/files/plan.pdf" {
		t.Fatalf("crawler metadata was not authoritative: %#v", response.Evidence)
	}
}

func TestKnowledgeSourceIdentityIsScopedBySchool(t *testing.T) {
	if sourceID("whut", "knowledge") == sourceID("hzau", "knowledge") {
		t.Fatal("the same upstream knowledge id must not collide across schools")
	}
}

type knowledgeMemoryCache struct{ values map[string][]byte }

func newKnowledgeMemoryCache() *knowledgeMemoryCache {
	return &knowledgeMemoryCache{values: make(map[string][]byte)}
}

func (c *knowledgeMemoryCache) GetJSON(_ context.Context, key string, target any) (bool, error) {
	value, ok := c.values[key]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(value, target)
}

func (c *knowledgeMemoryCache) SetJSON(_ context.Context, key string, value any, _ time.Duration) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.values[key] = encoded
	return nil
}

func TestGatewayCachesKnowledgeQueryBySchoolProviderAndKnowledgeBase(t *testing.T) {
	provider := &providerStub{}
	cache := newKnowledgeMemoryCache()
	gateway, err := NewGateway(provider, scopeStub{
		knowledgeBases: map[string]string{"whut": "kb-whut"}, names: map[string]string{"whut": "武汉理工大学"},
		domains: map[string][]string{"whut": {"whut.edu.cn"}},
	}, cache, CachePolicy{QueryTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	first, err := gateway.Search(context.Background(), Request{SchoolID: "whut", Query: " 奖学金   怎么评 ", TopN: 4})
	if err != nil {
		t.Fatal(err)
	}
	second, err := gateway.Search(context.Background(), Request{SchoolID: "whut", Query: "奖学金 怎么评", TopN: 4})
	if err != nil {
		t.Fatal(err)
	}
	if first.Stats.QueryCacheHit || !second.Stats.QueryCacheHit || provider.calls != 1 {
		t.Fatalf("knowledge query cache did not reuse provider result: first=%#v second=%#v calls=%d", first.Stats, second.Stats, provider.calls)
	}
}

func TestKnowledgeQueryCacheKeySeparatesRetrievalDepth(t *testing.T) {
	if queryCacheKey("whut", "v1", "weknora", "kb", "奖学金", 1) == queryCacheKey("whut", "v1", "weknora", "kb", "奖学金", 4) {
		t.Fatal("knowledge query cache must isolate different Top-N values")
	}
}

func TestKnowledgeQueryCacheKeySeparatesKnowledgeVersions(t *testing.T) {
	if queryCacheKey("whut", "v1", "weknora", "kb", "奖学金", 4) == queryCacheKey("whut", "v2", "weknora", "kb", "奖学金", 4) {
		t.Fatal("knowledge version bump must invalidate query cache")
	}
}

type failingKnowledgeCache struct{}

func (failingKnowledgeCache) GetJSON(context.Context, string, any) (bool, error) {
	return false, errors.New("redis read failed")
}

func (failingKnowledgeCache) SetJSON(context.Context, string, any, time.Duration) error {
	return errors.New("redis write failed")
}

func TestKnowledgeQueryCacheFailureFallsBackToProvider(t *testing.T) {
	provider := &providerStub{}
	gateway, err := NewGateway(provider, scopeStub{
		knowledgeBases: map[string]string{"whut": "kb-whut"}, names: map[string]string{"whut": "武汉理工大学"},
		domains: map[string][]string{"whut": {"whut.edu.cn"}},
	}, failingKnowledgeCache{}, CachePolicy{QueryTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	response, err := gateway.Search(context.Background(), Request{SchoolID: "whut", Query: "奖学金", TopN: 4})
	if err != nil || len(response.Evidence) != 1 || provider.calls != 1 || response.Stats.QueryCacheHit {
		t.Fatalf("cache failure blocked provider fallback: response=%#v calls=%d err=%v", response, provider.calls, err)
	}
}
