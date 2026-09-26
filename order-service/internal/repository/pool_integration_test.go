package repository_test

import (
	"context"
	"testing"
	"time"

	"order-service/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTinyPool(t *testing.T, acquireTimeout time.Duration) *repository.PoolConfig {
	t.Helper()
	return &repository.PoolConfig{MaxConns: 1, MinConns: 1, AcquireTimeout: acquireTimeout}
}

// Пул занят целиком: запрос не висит, а отказывает за время AcquireTimeout.
func TestPool_Exhausted_FailsFastWithDeadline(t *testing.T) {
	pool, err := repository.Connect(context.Background(), testDSN, zap.NewNop(), *newTinyPool(t, 200*time.Millisecond))
	require.NoError(t, err)
	defer pool.Close()

	held, err := pool.Acquire(context.Background())
	require.NoError(t, err)
	defer held.Release()

	start := time.Now()
	_, err = pool.Exec(context.Background(), "SELECT 1")
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.GreaterOrEqual(t, elapsed, 150*time.Millisecond, "ждали соединение")
	assert.Less(t, elapsed, time.Second, "но не дольше таймаута ожидания")
}

// Таймаут ограничивает только ожидание соединения: долгий запрос на
// свободном пуле не обрывается, даже если он дольше AcquireTimeout.
func TestPool_AcquireTimeout_DoesNotLimitQuery(t *testing.T) {
	pool, err := repository.Connect(context.Background(), testDSN, zap.NewNop(), *newTinyPool(t, 100*time.Millisecond))
	require.NoError(t, err)
	defer pool.Close()

	_, err = pool.Exec(context.Background(), "SELECT pg_sleep(0.3)")
	assert.NoError(t, err, "запрос дольше AcquireTimeout должен пройти")
}
