package agent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"
)

func TestCleaningAdmissionIgnoresLegacyAnswerCache(t *testing.T) {
	storage := newAnswerMemoryCache()
	question := "奖学金规则"
	digest := sha256.Sum256([]byte(question))
	for _, key := range []string{
		fmt.Sprintf("answer:whut:v1:%x", digest[:16]),
		fmt.Sprintf("answer:v2:whut:v1:zh-CN:%x", digest[:16]),
	} {
		if err := storage.SetJSON(context.Background(), key, verifiedCachedAnswer("旧准入答案"), time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	cache, err := NewVersionedAnswerCache(storage, versionResolverStub{"whut": "v1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, hit, err := cache.Lookup(context.Background(), "whut", question); err != nil || hit {
		t.Fatalf("legacy answer bypassed admission: hit=%v err=%v", hit, err)
	}
}
