package websearch

import (
	"context"
	"fmt"
	"time"
)

type MockProvider struct{}

func NewMockProvider() *MockProvider { return &MockProvider{} }
func (m *MockProvider) Name() string { return "mock-web-search" }
func (m *MockProvider) Search(ctx context.Context, request ProviderRequest) ([]SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	date := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	results := []SearchResult{
		{Title: "武汉理工大学本科生院", URL: "https://jwc.whut.edu.cn/", Snippet: "本科教学通知与办事入口。", Publisher: "武汉理工大学本科生院", PublishedAt: &date},
		{Title: "武汉理工大学官方网站", URL: "https://www.whut.edu.cn/", Snippet: "学校官方信息与部门入口。", Publisher: "武汉理工大学", PublishedAt: &date},
		{Title: "武汉理工大学图书馆", URL: "https://lib.whut.edu.cn/", Snippet: "图书馆开放与服务信息。", Publisher: "武汉理工大学图书馆", PublishedAt: &date},
	}
	if request.Limit > 0 && len(results) > request.Limit {
		results = results[:request.Limit]
	}
	return results, nil
}

type MockFetcher struct{}

func NewMockFetcher() *MockFetcher { return &MockFetcher{} }
func (m *MockFetcher) Fetch(ctx context.Context, rawURL string, scope Scope) (Page, error) {
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	if !IsAllowedURL(rawURL, scope.AllowedDomains) {
		return Page{}, ErrDisallowedURL
	}
	content := map[string]string{
		"https://jwc.whut.edu.cn/": `<html><body><h1>本科生院信息入口</h1><p>本页面用于 AskU 搜索联调，验证学校官方域名限制、页面抓取和相关片段提取。</p><p>具体校园政策、日期与办理流程必须以学校正式发布的原始通知为准。</p></body></html>`,
		"https://www.whut.edu.cn/": `<html><body><h1>武汉理工大学官方入口</h1><p>本页面用于验证多来源搜索与缓存，不代表真实政策通知。</p></body></html>`,
		"https://lib.whut.edu.cn/": `<html><body><h1>图书馆服务入口</h1><p>本页面用于验证 Top-N 页面抓取；正式开放时间应查看图书馆最新通知。</p></body></html>`,
	}[rawURL]
	if content == "" {
		return Page{}, fmt.Errorf("mock page not found: %s", rawURL)
	}
	return Page{URL: rawURL, ContentType: "text/html", Body: content, FetchedAt: time.Now().UTC()}, nil
}
