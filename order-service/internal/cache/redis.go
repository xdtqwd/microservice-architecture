package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrCacheMiss = errors.New("cache miss")

type RedisCache struct {
	client *redis.Client
}

func New(addr string) *RedisCache {
	// Кеш — ускоритель, а не зависимость: при проблемах с Redis
	// лучше быстро отказать и пойти в базу, чем держать запрос секундами.
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
		PoolTimeout:  100 * time.Millisecond,
		// -1: без повтора команд (0 в go-redis значит «по умолчанию», то есть 3)
		MaxRetries: -1,
		// пул сам повторяет подключение 5 раз с паузами — для кеша это
		// секунды ожидания на каждом запросе, пока Redis лежит
		DialerRetries: 1,
	})
	return &RedisCache{client: client}
}

func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

func (c *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	val, err := c.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return ErrCacheMiss
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(val), dest)
}

func (c *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}
