package repository_test

import (
	"testing"
	"time"

	"order-service/internal/cache"
	"order-service/internal/metrics"
	"order-service/internal/repository"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const deadRedis = "127.0.0.1:1"

func TestRedisDown_ReadFallsBackToDB(t *testing.T) {
	base := newCounting(7)
	l2 := repository.NewCachedProductRepo(base, cache.New(deadRedis), zap.NewNop())
	l1 := repository.NewL1ProductRepo(l2)

	getErrs := testutil.ToFloat64(metrics.CacheErrors.WithLabelValues("l2", "get"))
	setErrs := testutil.ToFloat64(metrics.CacheErrors.WithLabelValues("l2", "set"))

	start := time.Now()
	p, err := l1.GetProductByID(ctx, 1)
	elapsed := time.Since(start)

	require.NoError(t, err, "Redis лёг — это промах кеша, а не ошибка")
	assert.Equal(t, 7, p.Stock, "данные пришли из базы")
	assert.EqualValues(t, 1, base.calls.Load())
	assert.Less(t, elapsed, 500*time.Millisecond, "недоступный кеш не должен тормозить запрос")

	assert.Greater(t, testutil.ToFloat64(metrics.CacheErrors.WithLabelValues("l2", "get")), getErrs,
		"ошибка чтения видна в метрике")
	assert.Greater(t, testutil.ToFloat64(metrics.CacheErrors.WithLabelValues("l2", "set")), setErrs,
		"ошибка записи проглочена, но видна в метрике")
}

func TestRedisDown_InvalidateDoesNotFail(t *testing.T) {
	l2 := repository.NewCachedProductRepo(newCounting(7), cache.New(deadRedis), zap.NewNop())
	before := testutil.ToFloat64(metrics.CacheErrors.WithLabelValues("l2", "delete"))

	assert.NoError(t, l2.InvalidateByID(ctx, 1))
	assert.Greater(t, testutil.ToFloat64(metrics.CacheErrors.WithLabelValues("l2", "delete")), before)
}
