package cache

import (
	"strings"
	"testing"
)

func TestIdempotencyCacheKeyDoesNotExposeExternalRequestKey(t *testing.T) {
	requestKey := "partner-provided:key:with:user-data"
	key := idempotencyCacheKey("usr_123", requestKey)
	if strings.Contains(key, requestKey) || !strings.HasPrefix(key, "idem:usr_123:") {
		t.Fatalf("unsafe idempotency cache key: %q", key)
	}
	if key != idempotencyCacheKey("usr_123", requestKey) {
		t.Fatal("idempotency cache key must be deterministic")
	}
}

func TestIdempotencyCacheKeyScopesUsers(t *testing.T) {
	if idempotencyCacheKey("usr_a", "same") == idempotencyCacheKey("usr_b", "same") {
		t.Fatal("idempotency keys must be isolated by user")
	}
}
