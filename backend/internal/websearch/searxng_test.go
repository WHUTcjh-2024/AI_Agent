package websearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearXNGProviderMapsContractAndAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/search" || request.URL.Query().Get("q") != "校历 (site:whut.edu.cn)" || request.URL.Query().Get("format") != "json" {
			t.Fatalf("unexpected request: %s", request.URL.String())
		}
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("missing provider auth")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"results":[{"title":"校历","url":"https://jwc.whut.edu.cn/calendar","content":"官方校历","publishedDate":"2026-08-20"}]}`))
	}))
	defer server.Close()
	provider, err := NewSearXNGProvider(server.URL, "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	results, err := provider.Search(context.Background(), ProviderRequest{Query: "校历", AllowedDomains: []string{"whut.edu.cn"}, Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Publisher != "jwc.whut.edu.cn" || results[0].PublishedAt == nil {
		t.Fatalf("unexpected result: %#v", results)
	}
}
