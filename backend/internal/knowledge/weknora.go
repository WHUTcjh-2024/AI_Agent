package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxWeKnoraResponseBytes = 8 << 20

type WeKnoraProvider struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

func NewWeKnoraProvider(baseURL, apiKey string, client *http.Client) (*WeKnoraProvider, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("ASKU_WEKNORA_BASE_URL must be an absolute HTTP(S) URL without user info")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("ASKU_WEKNORA_API_KEY is required")
	}
	if client == nil {
		return nil, fmt.Errorf("WeKnora HTTP client must not be nil")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.RawPath = ""
	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(path, "/api/v1") {
		parsed.Path = path + "/knowledge-search"
	} else {
		parsed.Path = path + "/api/v1/knowledge-search"
	}
	return &WeKnoraProvider{endpoint: parsed.String(), apiKey: strings.TrimSpace(apiKey), client: client}, nil
}

func (p *WeKnoraProvider) Name() string { return "weknora" }

func (p *WeKnoraProvider) Search(ctx context.Context, request ProviderRequest) ([]Evidence, error) {
	if strings.TrimSpace(request.Query) == "" || len(request.KnowledgeBaseIDs) == 0 {
		return nil, fmt.Errorf("WeKnora search requires a query and at least one knowledge base")
	}
	body, err := json.Marshal(map[string]any{
		"query": request.Query, "knowledge_base_ids": request.KnowledgeBaseIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("encode WeKnora request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create WeKnora request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("X-API-Key", p.apiKey)
	if request.RequestID != "" {
		httpRequest.Header.Set("X-Request-ID", request.RequestID)
	}
	response, err := p.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("call WeKnora knowledge search: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxWeKnoraResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read WeKnora response: %w", err)
	}
	if len(data) > maxWeKnoraResponseBytes {
		return nil, fmt.Errorf("WeKnora response exceeds %d bytes", maxWeKnoraResponseBytes)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("WeKnora knowledge search returned HTTP %d", response.StatusCode)
	}
	var envelope weKnoraSearchResponse
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode WeKnora response: %w", err)
	}
	if !envelope.Success {
		message := "request failed"
		if envelope.Error != nil && strings.TrimSpace(envelope.Error.Message) != "" {
			message = envelope.Error.Message
		}
		return nil, fmt.Errorf("WeKnora knowledge search: %s", message)
	}
	limit := request.Limit
	if limit < 1 || limit > len(envelope.Data) {
		limit = len(envelope.Data)
	}
	result := make([]Evidence, 0, limit)
	for _, item := range envelope.Data[:limit] {
		if strings.TrimSpace(item.KnowledgeID) == "" || strings.TrimSpace(item.Content) == "" {
			continue
		}
		result = append(result, Evidence{
			ChunkID: item.ID, KnowledgeID: item.KnowledgeID, Title: firstNonEmpty(item.KnowledgeTitle, item.KnowledgeFilename),
			Filename: item.KnowledgeFilename, Content: item.Content, Score: item.Score, Metadata: item.Metadata,
			SourceURL:   metadataString(item.Metadata, "source_url", "url", "origin_url"),
			Publisher:   metadataString(item.Metadata, "publisher", "department", "source_department"),
			PublishedAt: metadataTime(item.Metadata, "published_at", "publish_date", "publishedAt"),
		})
	}
	return result, nil
}

type weKnoraSearchResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		ID                string         `json:"id"`
		Content           string         `json:"content"`
		KnowledgeID       string         `json:"knowledge_id"`
		KnowledgeTitle    string         `json:"knowledge_title"`
		KnowledgeFilename string         `json:"knowledge_filename"`
		Score             float64        `json:"score"`
		Metadata          map[string]any `json:"metadata"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func metadataString(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := metadata[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func metadataTime(metadata map[string]any, keys ...string) *time.Time {
	value := metadataString(metadata, keys...)
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006/01/02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "未命名学校资料"
}
