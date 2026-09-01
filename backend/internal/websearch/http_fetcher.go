package websearch

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultMaxPageBytes int64 = 2 << 20

type HTTPFetcher struct {
	client   *http.Client
	maxBytes int64
}

func NewHTTPFetcher(client *http.Client, maxBytes int64) *HTTPFetcher {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxPageBytes
	}
	return &HTTPFetcher{client: client, maxBytes: maxBytes}
}

func (f *HTTPFetcher) Fetch(ctx context.Context, rawURL string, scope Scope) (Page, error) {
	if !IsAllowedURL(rawURL, scope.AllowedDomains) {
		return Page{}, ErrDisallowedURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Page{}, fmt.Errorf("create page request: %w", err)
	}
	request.Header.Set("Accept", "text/html,text/plain;q=0.9")
	request.Header.Set("User-Agent", "AskU-WebSearch/0.1")

	client := *f.client
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 4 || !IsAllowedURL(next.URL.String(), scope.AllowedDomains) {
			return ErrDisallowedURL
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		return Page{}, fmt.Errorf("fetch page: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Page{}, fmt.Errorf("fetch page: unexpected HTTP status %d", response.StatusCode)
	}
	contentType := response.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "" {
		mediaType = inferMediaType(response.Request.URL)
	}
	if mediaType != "text/html" && mediaType != "text/plain" {
		return Page{}, fmt.Errorf("%w: %s", ErrUnsupportedMedia, mediaType)
	}
	limited := io.LimitReader(response.Body, f.maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return Page{}, fmt.Errorf("read page: %w", err)
	}
	if int64(len(body)) > f.maxBytes {
		return Page{}, fmt.Errorf("page exceeds %d bytes", f.maxBytes)
	}
	return Page{URL: response.Request.URL.String(), ContentType: mediaType, Body: string(body), FetchedAt: time.Now().UTC()}, nil
}

func inferMediaType(parsed *url.URL) string {
	path := strings.ToLower(parsed.Path)
	if strings.HasSuffix(path, ".txt") {
		return "text/plain"
	}
	return "text/html"
}

func hostLabel(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "学校官网"
	}
	if host := parsed.Hostname(); host != "" {
		return host
	}
	return "学校官网"
}
