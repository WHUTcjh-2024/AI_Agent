package websearch

import (
	"net/url"
	"strings"
)

func IsAllowedURL(rawURL string, allowedDomains []string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" {
		return false
	}
	for _, candidate := range allowedDomains {
		domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(candidate)), ".")
		if domain != "" && (host == domain || strings.HasSuffix(host, "."+domain)) {
			return true
		}
	}
	return false
}
