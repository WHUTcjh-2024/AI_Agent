package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"asku/backend/internal/domain"
)

type answerMemoryCache struct {
	values map[string][]byte
	keys   []string
}

func newAnswerMemoryCache() *answerMemoryCache {
	return &answerMemoryCache{values: make(map[string][]byte)}
}

func (c *answerMemoryCache) GetJSON(_ context.Context, key string, target any) (bool, error) {
	value, ok := c.values[key]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(value, target)
}

func (c *answerMemoryCache) SetJSON(_ context.Context, key string, value any, _ time.Duration) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.values[key] = encoded
	c.keys = append(c.keys, key)
	return nil
}

type versionResolverStub map[string]string

func (v versionResolverStub) KnowledgeVersion(schoolID string) (string, error) {
	version, ok := v[schoolID]
	if !ok {
		return "", fmt.Errorf("unknown school")
	}
	return version, nil
}

func TestVersionedAnswerCacheNormalizesQuestionAndScopesSchool(t *testing.T) {
	storage := newAnswerMemoryCache()
	cache, err := NewVersionedAnswerCache(storage, versionResolverStub{"whut": "v1", "hzau": "v1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	answer := verifiedCachedAnswer("已核验答案")
	if err := cache.Store(context.Background(), "whut", "  奖学金   怎么评？ ", answer); err != nil {
		t.Fatal(err)
	}
	cached, hit, err := cache.Lookup(context.Background(), "whut", "奖学金 怎么评？")
	if err != nil || !hit || cached.Answer != answer.Answer {
		t.Fatalf("normalized query missed cache: hit=%v answer=%#v err=%v", hit, cached, err)
	}
	if _, hit, err := cache.Lookup(context.Background(), "hzau", "奖学金 怎么评？"); err != nil || hit {
		t.Fatalf("answer cache leaked across schools: hit=%v err=%v", hit, err)
	}
}

func TestVersionedAnswerCacheDoesNotStoreUnverifiedAnswer(t *testing.T) {
	storage := newAnswerMemoryCache()
	cache, err := NewVersionedAnswerCache(storage, versionResolverStub{"whut": "v1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Store(context.Background(), "whut", "问题", CachedAnswer{Answer: "没有来源"}); err != nil {
		t.Fatal(err)
	}
	if len(storage.values) != 0 {
		t.Fatalf("unverified answer entered cache: %#v", storage.keys)
	}
}

func TestAnswerCacheVersionBumpInvalidatesOldEntry(t *testing.T) {
	storage := newAnswerMemoryCache()
	versions := versionResolverStub{"whut": "v1"}
	cache, err := NewVersionedAnswerCache(storage, versions, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	answer := verifiedCachedAnswer("旧答案")
	if err := cache.Store(context.Background(), "whut", "校历", answer); err != nil {
		t.Fatal(err)
	}
	versions["whut"] = "v2"
	if _, hit, err := cache.Lookup(context.Background(), "whut", "校历"); err != nil || hit {
		t.Fatalf("knowledge version bump did not invalidate entry: hit=%v err=%v", hit, err)
	}
}

func verifiedCachedAnswer(answer string) CachedAnswer {
	return CachedAnswer{
		Answer:    answer,
		Sources:   []domain.Source{{ID: "src-1", Official: true}},
		Citations: []domain.Citation{{CitationID: "cit-1", Index: 1, SourceID: "src-1", Title: "正式通知", EvidenceText: "已核验内容", Authority: "OFFICIAL_DEPARTMENT", OfficialURL: "https://www.whut.edu.cn"}},
	}
}
