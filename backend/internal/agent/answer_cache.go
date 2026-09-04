package agent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

// AnswerJSONCache is implemented by Redis without exposing Redis types to the
// agent package.
type AnswerJSONCache interface {
	GetJSON(ctx context.Context, key string, target any) (bool, error)
	SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
}

// KnowledgeVersionResolver keeps answer invalidation school-scoped. Bumping a
// school's version makes old answers unreachable without a broad key delete.
type KnowledgeVersionResolver interface {
	KnowledgeVersion(schoolID string) (string, error)
}

type VersionedAnswerCache struct {
	cache    AnswerJSONCache
	versions KnowledgeVersionResolver
	ttl      time.Duration
}

func NewVersionedAnswerCache(cache AnswerJSONCache, versions KnowledgeVersionResolver, ttl time.Duration) (*VersionedAnswerCache, error) {
	if cache == nil || versions == nil {
		return nil, fmt.Errorf("answer cache dependencies must not be nil")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("answer cache TTL must be positive")
	}
	return &VersionedAnswerCache{cache: cache, versions: versions, ttl: ttl}, nil
}

func (c *VersionedAnswerCache) Lookup(ctx context.Context, schoolID, question string) (CachedAnswer, bool, error) {
	key, err := c.key(schoolID, question)
	if err != nil {
		return CachedAnswer{}, false, err
	}
	var answer CachedAnswer
	hit, err := c.cache.GetJSON(ctx, key, &answer)
	if err != nil || !hit {
		return CachedAnswer{}, false, err
	}
	if !isCacheableAnswer(answer) {
		return CachedAnswer{}, false, nil
	}
	return answer, true, nil
}

func (c *VersionedAnswerCache) Store(ctx context.Context, schoolID, question string, answer CachedAnswer) error {
	if !isCacheableAnswer(answer) {
		return nil
	}
	key, err := c.key(schoolID, question)
	if err != nil {
		return err
	}
	return c.cache.SetJSON(ctx, key, answer, c.ttl)
}

func (c *VersionedAnswerCache) key(schoolID, question string) (string, error) {
	schoolID = strings.TrimSpace(schoolID)
	normalizedQuestion := strings.ToLower(strings.Join(strings.Fields(question), " "))
	if schoolID == "" || normalizedQuestion == "" {
		return "", fmt.Errorf("answer cache requires school and question")
	}
	version, err := c.versions.KnowledgeVersion(schoolID)
	if err != nil {
		return "", err
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return "", fmt.Errorf("school %q has no knowledge version", schoolID)
	}
	digest := sha256.Sum256([]byte(normalizedQuestion))
	// Discard answers produced before content-bound cleaning admission existed.
	version = "admission-v1-" + version
	return fmt.Sprintf("answer:%s:%s:%x", cacheKeySegment(schoolID), cacheKeySegment(version), digest[:16]), nil
}

func cacheKeySegment(value string) string {
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' {
			continue
		}
		digest := sha256.Sum256([]byte(value))
		return fmt.Sprintf("h%x", digest[:8])
	}
	return value
}
