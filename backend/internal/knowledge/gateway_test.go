package knowledge

import (
	"context"
	"fmt"
	"testing"
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
	})
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
	})
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
	})
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
	})
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
	})
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

func TestKnowledgeSourceIdentityIsScopedBySchool(t *testing.T) {
	if sourceID("whut", "knowledge") == sourceID("hzau", "knowledge") {
		t.Fatal("the same upstream knowledge id must not collide across schools")
	}
}
