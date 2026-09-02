package knowledge

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
)

type Gateway struct {
	provider Provider
	scopes   ScopeResolver
	cache    JSONCache
	policy   CachePolicy
	catalog  Catalog
}

func NewGateway(provider Provider, scopes ScopeResolver, cache JSONCache, policy CachePolicy, catalogs ...Catalog) (*Gateway, error) {
	if provider == nil {
		return nil, fmt.Errorf("knowledge provider must not be nil")
	}
	if scopes == nil {
		return nil, fmt.Errorf("knowledge scope resolver must not be nil")
	}
	if policy.QueryTTL <= 0 {
		return nil, fmt.Errorf("knowledge query cache TTL must be positive")
	}
	var catalog Catalog
	if len(catalogs) > 0 {
		catalog = catalogs[0]
	}
	return &Gateway{provider: provider, scopes: scopes, cache: cache, policy: policy, catalog: catalog}, nil
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
	knowledgeVersion, err := g.scopes.KnowledgeVersion(request.SchoolID)
	if err != nil {
		return Response{}, err
	}
	cacheKey := queryCacheKey(request.SchoolID, knowledgeVersion, g.provider.Name(), knowledgeBaseID, query, request.TopN)
	var evidence []Evidence
	cacheHit := false
	if g.cache != nil {
		cacheHit, err = g.cache.GetJSON(ctx, cacheKey, &evidence)
		if err != nil {
			slog.Warn("read knowledge query cache", "provider", g.provider.Name(), "error", err)
			cacheHit = false
			evidence = nil
		}
	}
	if !cacheHit {
		evidence, err = g.provider.Search(ctx, ProviderRequest{
			Query: query, KnowledgeBaseIDs: []string{knowledgeBaseID}, RequestID: request.RunID, Limit: request.TopN,
		})
		if err != nil {
			return Response{}, err
		}
	}
	normalized := make([]Evidence, 0, len(evidence))
	for _, item := range evidence {
		item.KnowledgeID = strings.TrimSpace(item.KnowledgeID)
		item.ChunkID = strings.TrimSpace(item.ChunkID)
		item.Content = strings.TrimSpace(item.Content)
		if item.KnowledgeID == "" || strings.TrimSpace(item.ChunkID) == "" || item.Content == "" {
			continue
		}
		if g.catalog != nil {
			metadata, found, resolveErr := g.catalog.ResolveEvidence(ctx, request.SchoolID, item.KnowledgeID)
			if resolveErr != nil {
				return Response{}, fmt.Errorf("resolve knowledge evidence: %w", resolveErr)
			}
			if !found {
				// A WeKnora hit without an AskU crawler mapping cannot produce a
				// verifiable public citation, so it cannot ground an answer.
				continue
			}
			applyDocumentMetadata(&item, metadata)
		}
		item.SourceID = sourceID(request.SchoolID, firstPresent(item.AskUDocumentID, item.KnowledgeID))
		item.OfficialURL = allowedSourceURL(item.OfficialURL, allowedDomains)
		item.AttachmentURL = allowedSourceURL(item.AttachmentURL, allowedDomains)
		item.ParentPageURL = allowedSourceURL(item.ParentPageURL, allowedDomains)
		publicAttachments := make([]Attachment, 0, len(item.Attachments))
		for _, attachment := range item.Attachments {
			attachment.URL = allowedSourceURL(attachment.URL, allowedDomains)
			attachment.ParentPageURL = allowedSourceURL(attachment.ParentPageURL, allowedDomains)
			if attachment.URL != "" {
				publicAttachments = append(publicAttachments, attachment)
			}
		}
		item.Attachments = publicAttachments
		providerURL := ""
		if g.catalog == nil {
			providerURL = allowedSourceURL(item.SourceURL, allowedDomains)
		}
		item.SourceURL = firstPresent(item.AttachmentURL, item.OfficialURL, item.ParentPageURL, providerURL)
		if g.catalog != nil && item.SourceURL == "" {
			continue
		}
		item.Title = truncateText(strings.TrimSpace(item.Title), 300)
		item.Filename = truncateText(strings.TrimSpace(item.Filename), 300)
		item.Publisher = strings.TrimSpace(item.Publisher)
		if item.Publisher == "" {
			item.Publisher = schoolName
		}
		item.Publisher = truncateText(item.Publisher, 160)
		item.Metadata = nil
		normalized = append(normalized, item)
		if len(normalized) == request.TopN {
			break
		}
	}
	if !cacheHit && g.cache != nil {
		if err := g.cache.SetJSON(ctx, cacheKey, normalized, g.policy.QueryTTL); err != nil {
			slog.Warn("write knowledge query cache", "provider", g.provider.Name(), "error", err)
		}
	}
	return Response{
		Evidence: normalized,
		Stats: Stats{
			Provider: g.provider.Name(), Configured: true, Hits: len(normalized), QueryCacheHit: cacheHit,
		},
	}, nil
}

func applyDocumentMetadata(item *Evidence, metadata DocumentMetadata) {
	item.AskUDocumentID = strings.TrimSpace(metadata.AskUDocumentID)
	item.Title = firstPresent(metadata.Title, item.Title)
	item.Publisher = firstPresent(metadata.SourceName, metadata.Department, item.Publisher)
	item.Department = firstPresent(metadata.Department, metadata.SourceName)
	item.PublishedAt = metadata.PublishedAt
	item.SourceType = strings.TrimSpace(metadata.SourceType)
	item.DocumentType = strings.TrimSpace(metadata.DocumentType)
	item.OfficialURL = firstPresent(metadata.OfficialURL, metadata.CanonicalURL)
	item.AttachmentURL = strings.TrimSpace(metadata.AttachmentURL)
	item.ParentPageURL = strings.TrimSpace(metadata.ParentPageURL)
	item.Authority = firstPresent(metadata.Authority, "OFFICIAL_DEPARTMENT")
	item.Freshness = strings.TrimSpace(metadata.Freshness)
	item.KnowledgeBundleID = strings.TrimSpace(metadata.KnowledgeBundleID)
	item.Attachments = metadata.Attachments
}

func firstPresent(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func queryCacheKey(schoolID, knowledgeVersion, provider, knowledgeBaseID, query string, topN int) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(query), " "))
	material := strings.Join([]string{strings.TrimSpace(knowledgeVersion), strings.TrimSpace(provider), strings.TrimSpace(knowledgeBaseID), normalized, fmt.Sprint(topN)}, "\x00")
	digest := sha256.Sum256([]byte(material))
	return fmt.Sprintf("query:%s:%x", strings.TrimSpace(schoolID), digest[:16])
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
