package websearch

import (
	"context"
	"strings"
	"testing"
)

func TestHTMLExtractorPrefersRelevantVisibleContent(t *testing.T) {
	page := Page{URL: "https://jwc.whut.edu.cn/a", ContentType: "text/html", Body: `<html><body><nav>转专业导航噪声</nav><p>普通校园介绍内容，不包含办理信息。</p><h2>转专业申请安排</h2><p>转专业申请应查看本科生院发布的最新通知。</p><script>转专业 secret</script></body></html>`}
	sections, err := NewHTMLExtractor(2, 200).ExtractRelevantSections(context.Background(), "转专业申请", page)
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) == 0 || !strings.Contains(sections[0].Text, "转专业") {
		t.Fatalf("unexpected sections: %#v", sections)
	}
	for _, section := range sections {
		if strings.Contains(section.Text, "secret") || strings.Contains(section.Text, "导航噪声") {
			t.Fatalf("hidden/noisy content leaked: %#v", sections)
		}
	}
}
