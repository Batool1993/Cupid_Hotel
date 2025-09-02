package redisad

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"cupid_hotel/internal/adapters/observability"
)

type Cache struct{ c *redis.Client }

func New(addr, pass string, db int) *Cache {
	return &Cache{c: redis.NewClient(&redis.Options{Addr: addr, Password: pass, DB: db})}
}

func (r *Cache) Get(ctx context.Context, key string, dst any) (bool, error) {
	v, err := r.c.Get(ctx, key).Bytes()
	if err == redis.Nil {
		observability.ObserveCache("redis", "miss")
		return false, nil
	}
	if err != nil {
		return false, err
	}
	observability.ObserveCache("redis", "hit")
	return true, json.Unmarshal(v, dst)
}

func (r *Cache) Set(ctx context.Context, key string, v any, ttlSec int) error {
	b, _ := json.Marshal(v)
	observability.ObserveCache("redis", "set")
	return r.c.Set(ctx, key, b, time.Duration(ttlSec)*time.Second).Err()
}

func (r *Cache) Del(ctx context.Context, key string) error {
	observability.ObserveCache("redis", "del")
	return r.c.Del(ctx, key).Err()
}
