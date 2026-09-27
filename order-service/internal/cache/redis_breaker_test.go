package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Мёртвый Redis: после threshold ошибок клиент перестаёт ходить в сеть
// и отвечает ErrUnavailable мгновенно.
func TestRedisCache_BreakerOpensAndSkipsNetwork(t *testing.T) {
	c := New("127.0.0.1:1", WithBreaker(3, time.Minute))
	ctx := context.Background()
	var v string

	for i := 0; i < 3; i++ {
		err := c.Get(ctx, "k", &v)
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrUnavailable, "первые вызовы реально ходят в Redis")
	}

	start := time.Now()
	for i := 0; i < 100; i++ {
		assert.ErrorIs(t, c.Get(ctx, "k", &v), ErrUnavailable)
		assert.ErrorIs(t, c.Set(ctx, "k", "v", time.Second), ErrUnavailable)
	}
	assert.Less(t, time.Since(start), 50*time.Millisecond, "200 вызовов без сети")

	assert.NotErrorIs(t, c.Delete(ctx, "k"), ErrUnavailable,
		"инвалидация идёт мимо предохранителя")
}
