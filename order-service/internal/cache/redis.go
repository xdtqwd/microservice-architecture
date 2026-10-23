package cache

import (
	"context"
	"encoding/json"
	"errors"
	"order-service/internal/breaker"
	"order-service/internal/metrics"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrCacheMiss = errors.New("cache miss")

// ErrUnavailable — вызов не делался: предохранитель разомкнут, Redis считаем лежащим.
var ErrUnavailable = errors.New("cache unavailable: circuit open")

type RedisCache struct {
	client  *redis.Client
	breaker *breaker.Breaker
}

type Option func(*RedisCache)

// WithBreaker ставит предохранитель на чтение и запись в кеш.
func WithBreaker(threshold int, openFor time.Duration) Option {
	return func(c *RedisCache) {
		c.breaker = breaker.New("redis", threshold, openFor,
			breaker.WithOnChange(func(name string, from, to breaker.State) {
				metrics.BreakerState.WithLabelValues(name).Set(float64(to))
				metrics.BreakerTransitions.WithLabelValues(name, from.String(), to.String()).Inc()
			}))
		metrics.BreakerState.WithLabelValues("redis").Set(0)
	}
}

func New(addr string, opts ...Option) *RedisCache {
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
	c := &RedisCache{client: client}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

func (c *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	var val string
	err := c.guard(ctx, func() error {
		var e error
		val, e = c.client.Get(ctx, key).Result()
		return e
	})
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
	return c.guard(ctx, func() error {
		return c.client.Set(ctx, key, data, ttl).Err()
	})
}

// guard пропускает вызов через предохранитель, если он включён.
// Отказом Redis считаем только ошибку самого Redis: промах кеша (redis.Nil)
// и отмена контекста клиентом — не его вина.
func (c *RedisCache) guard(ctx context.Context, call func() error) error {
	if c.breaker == nil {
		return call()
	}
	if err := c.breaker.Allow(); err != nil {
		return ErrUnavailable
	}
	err := call()
	c.breaker.Done(!isRedisFailure(ctx, err))
	return err
}

// isRedisFailure — отказал ли сам Redis. Промах кеша (redis.Nil) — нормальный
// ответ, а отмена контекста клиентом — не вина Redis: ни то, ни другое
// не должно размыкать предохранитель.
func isRedisFailure(ctx context.Context, err error) bool {
	switch {
	case err == nil, errors.Is(err, redis.Nil):
		return false
	case errors.Is(err, context.Canceled) && ctx.Err() != nil:
		return false
	default:
		return true
	}
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}
