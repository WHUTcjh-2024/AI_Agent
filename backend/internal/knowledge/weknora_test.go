package knowledge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWeKnoraProviderUsesOfficialKnowledgeSearchContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/knowledge-search" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-API-Key") != "secret" || request.Header.Get("X-Request-ID") != "run-1" {
			t.Fatalf("required WeKnora headers are missing")
		}
		var body struct {
			Query            string   `json:"query"`
			KnowledgeBaseIDs []string `json:"knowledge_base_ids"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Query != "奖学金" || len(body.KnowledgeBaseIDs) != 1 || body.KnowledgeBaseIDs[0] != "kb-whut" {
			t.Fatalf("unexpected WeKnora request body: %#v", body)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{
			"success": true,
			"data": [{
				"id": "chunk-1",
				"content": "奖学金评定依据学校正式文件。",
				"knowledge_id": "knowledge-1",
				"knowledge_title": "奖学金评定办法",
				"knowledge_filename": "award.pdf",
				"score": 0.95,
				"metadata": {
					"source_url": "https://whut.edu.cn/award",
					"publisher": "学生工作部",
					"published_at": "2026-08-20"
				}
			}]
		}`))
	}))
	defer server.Close()

	provider, err := NewWeKnoraProvider(server.URL, "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := provider.Search(context.Background(), ProviderRequest{
		Query: "奖学金", KnowledgeBaseIDs: []string{"kb-whut"}, RequestID: "run-1", Limit: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 || evidence[0].KnowledgeID != "knowledge-1" || evidence[0].SourceURL != "https://whut.edu.cn/award" {
		t.Fatalf("unexpected evidence mapping: %#v", evidence)
	}
	if evidence[0].PublishedAt == nil || !evidence[0].PublishedAt.Equal(time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("published date was not mapped: %#v", evidence[0].PublishedAt)
	}
}

func TestWeKnoraProviderRejectsUnsafeOrIncompleteConfiguration(t *testing.T) {
	for _, testCase := range []struct {
		baseURL string
		apiKey  string
	}{
		{baseURL: "localhost:8080", apiKey: "secret"},
		{baseURL: "https://user:pass@example.com", apiKey: "secret"},
		{baseURL: "https://example.com", apiKey: ""},
	} {
		if _, err := NewWeKnoraProvider(testCase.baseURL, testCase.apiKey, &http.Client{}); err == nil {
			t.Fatalf("invalid configuration was accepted: %#v", testCase)
		}
	}
}
