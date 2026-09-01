package websearch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHTTPFetcherReadsAllowedHTMLAndRejectsOversize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte("<p>学校官方资料</p>"))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	scope := Scope{AllowedDomains: []string{parsed.Hostname()}}

	page, err := NewHTTPFetcher(&http.Client{Timeout: time.Second}, 100).Fetch(context.Background(), server.URL, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Body, "学校官方资料") {
		t.Fatalf("unexpected body: %s", page.Body)
	}
	if _, err := NewHTTPFetcher(&http.Client{Timeout: time.Second}, 5).Fetch(context.Background(), server.URL, scope); err == nil {
		t.Fatal("expected size error")
	}
}

func TestHTTPFetcherRejectsRedirectOutsideAllowlist(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer target.Close()
	targetURL, _ := url.Parse(target.URL)
	disallowedTarget := "http://localhost:" + targetURL.Port()
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, disallowedTarget, http.StatusFound)
	}))
	defer redirect.Close()
	redirectURL, _ := url.Parse(redirect.URL)
	_, err := NewHTTPFetcher(&http.Client{Timeout: time.Second}, 100).Fetch(context.Background(), redirect.URL, Scope{AllowedDomains: []string{redirectURL.Hostname()}})
	if err == nil || !errors.Is(err, ErrDisallowedURL) {
		t.Fatalf("got %v, want ErrDisallowedURL", err)
	}
}
