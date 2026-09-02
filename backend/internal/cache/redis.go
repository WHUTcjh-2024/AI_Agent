package cache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	client *redis.Client
}

func Open(ctx context.Context, addr, password string) (*Redis, error) {
	client := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: 0, DialTimeout: 2 * time.Second})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &Redis{client: client}, nil
}

func (r *Redis) Close() error { return r.client.Close() }

func (r *Redis) Ping(ctx context.Context) error { return r.client.Ping(ctx).Err() }

// GetJSON and SetJSON form the small cache boundary used by feature modules.
// Callers own key design and value types; infrastructure only owns Redis I/O.
func (r *Redis) GetJSON(ctx context.Context, key string, target any) (bool, error) {
	value, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get cache value: %w", err)
	}
	if err := json.Unmarshal(value, target); err != nil {
		return false, fmt.Errorf("decode cache value: %w", err)
	}
	return true, nil
}

func (r *Redis) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode cache value: %w", err)
	}
	if err := r.client.Set(ctx, key, encoded, ttl).Err(); err != nil {
		return fmt.Errorf("set cache value: %w", err)
	}
	return nil
}

func (r *Redis) AllowQuestion(ctx context.Context, userID string, limit int64) (bool, error) {
	now := time.Now().UTC()
	key := fmt.Sprintf("rate:user:%s:%s", userID, now.Format("200601021504"))
	pipe := r.client.TxPipeline()
	count := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, 2*time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return count.Val() <= limit, nil
}

func (r *Redis) ReserveIdempotency(ctx context.Context, userID, key string) (bool, error) {
	if key == "" {
		return true, nil
	}
	return r.client.SetNX(ctx, idempotencyCacheKey(userID, key), "1", 10*time.Minute).Result()
}

func (r *Redis) ReleaseIdempotency(ctx context.Context, userID, key string) error {
	if key == "" {
		return nil
	}
	return r.client.Del(ctx, idempotencyCacheKey(userID, key)).Err()
}

func idempotencyCacheKey(userID, requestKey string) string {
	digest := sha256.Sum256([]byte(requestKey))
	return fmt.Sprintf("idem:%s:%x", userID, digest[:16])
}
