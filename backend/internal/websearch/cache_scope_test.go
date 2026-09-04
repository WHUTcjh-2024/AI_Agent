package websearch

import "testing"

func TestAllWebCacheLayersAreSchoolScoped(t *testing.T) {
	url, query := "https://shared.example.edu.cn/rule", "same question"
	if searchCacheKey("whut", query) == searchCacheKey("testu", query) ||
		pageCacheKey("whut", url) == pageCacheKey("testu", url) ||
		extractCacheKey("whut", query, url) == extractCacheKey("testu", query, url) {
		t.Fatal("web cache scope collision")
	}
}
