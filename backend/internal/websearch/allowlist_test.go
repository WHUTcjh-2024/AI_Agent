package websearch

import "testing"

func TestIsAllowedURL(t *testing.T) {
	allowed := []string{"whut.edu.cn"}
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"root", "https://whut.edu.cn/news", true},
		{"subdomain", "https://jwc.whut.edu.cn/news", true},
		{"evil suffix", "https://whut.edu.cn.evil.example/news", false},
		{"userinfo", "https://whut.edu.cn@evil.example/news", false},
		{"unsupported scheme", "file:///etc/passwd", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsAllowedURL(test.url, allowed); got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}
