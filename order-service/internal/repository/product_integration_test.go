package repository_test

import (
	"context"
	"testing"

	"order-service/internal/cache"
	"order-service/internal/domain"
	"order-service/internal/repository"
	"order-service/internal/txm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestProductRepo_GetProductsAndByID(t *testing.T) {
	resetDB(t)
	a := seedProduct(t, "A", 100, 10)
	seedProduct(t, "B", 200, 5)
	repo := repository.NewProductRepo(testPool)
	ctx := context.Background()

	all, err := repo.GetProducts(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	p, err := repo.GetProductByID(ctx, a)
	require.NoError(t, err)
	assert.Equal(t, "A", p.Name)
	assert.Equal(t, 10, p.Stock)

	_, err = repo.GetProductByID(ctx, 99999)
	assert.ErrorIs(t, err, domain.ErrProductNotFound)

	assert.NoError(t, repo.InvalidateByID(ctx, a))
}

func TestOutbox_InsertOutsideTx(t *testing.T) {
	resetDB(t)
	repo := repository.NewOutboxRepo(testPool)

	err := repo.Insert(context.Background(), 42, "order_created", map[string]int{"order_id": 42})
	require.NoError(t, err)

	var aggID int
	var payload string
	require.NoError(t, testPool.QueryRow(context.Background(),
		"SELECT aggregate_id, payload::text FROM outbox").Scan(&aggID, &payload))
	assert.Equal(t, 42, aggID)
	assert.JSONEq(t, `{"order_id": 42}`, payload)
}

func TestOutbox_RolledBackWithTransaction(t *testing.T) {
	resetDB(t)
	repo := repository.NewOutboxRepo(testPool)
	tm := txm.New(testPool)

	err := tm.Do(context.Background(), func(ctx context.Context) error {
		require.NoError(t, repo.Insert(ctx, 1, "order_created", map[string]int{"order_id": 1}))
		return assert.AnError // откат
	})

	assert.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, 0, countRows(t, "outbox"), "событие откатилось вместе с транзакцией")
}

func newCached() *repository.CachedProductRepo {
	return repository.NewCachedProductRepo(
		repository.NewProductRepo(testPool),
		cache.New(testRedisAddr),
		zap.NewNop())
}

func TestCachedRepo_SecondReadFromRedis(t *testing.T) {
	resetDB(t)
	pid := seedProduct(t, "A", 100, 10)
	repo := newCached()
	ctx := context.Background()

	_, err := repo.GetProductByID(ctx, pid)
	require.NoError(t, err)
	_, err = repo.GetProductByID(ctx, pid)
	require.NoError(t, err)

	assert.EqualValues(t, 1, repo.DBCalls(), "второй запрос обслужен из Redis")
}

func TestCachedRepo_InvalidateForcesDBRead(t *testing.T) {
	resetDB(t)
	pid := seedProduct(t, "A", 100, 10)
	repo := newCached()
	ctx := context.Background()

	_, _ = repo.GetProductByID(ctx, pid)
	_, _ = testPool.Exec(ctx, "UPDATE products SET stock = 3 WHERE id = $1", pid)
	require.NoError(t, repo.InvalidateByID(ctx, pid))

	p, err := repo.GetProductByID(ctx, pid)
	require.NoError(t, err)
	assert.Equal(t, 3, p.Stock, "после инвалидации — свежий остаток из БД")
	assert.EqualValues(t, 2, repo.DBCalls())
}

func TestCachedRepo_NotFoundAndGetProducts(t *testing.T) {
	resetDB(t)
	seedProduct(t, "A", 100, 10)
	repo := newCached()
	ctx := context.Background()

	_, err := repo.GetProductByID(ctx, 99999)
	assert.ErrorIs(t, err, domain.ErrProductNotFound)

	all, err := repo.GetProducts(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

// ---------- L1ProductRepo (память поверх Redis) ----------

func TestL1Repo_ServesFromMemoryAndInvalidates(t *testing.T) {
	resetDB(t)
	pid := seedProduct(t, "A", 100, 10)
	l2 := newCached()
	l1 := repository.NewL1ProductRepo(l2)
	ctx := context.Background()

	_, err := l1.GetProductByID(ctx, pid)
	require.NoError(t, err)
	require.NoError(t, testRedis.FlushAll(ctx).Err())
	_, err = l1.GetProductByID(ctx, pid)
	require.NoError(t, err)
	assert.EqualValues(t, 1, l2.DBCalls(), "второй запрос обслужен L1")

	require.NoError(t, l1.InvalidateByID(ctx, pid))
	_, err = l1.GetProductByID(ctx, pid)
	require.NoError(t, err)
	assert.EqualValues(t, 2, l2.DBCalls(), "после инвалидации L1 идёт вниз по стеку")

	all, err := l1.GetProducts(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)

	_, err = l1.GetProductByID(ctx, 99999)
	assert.ErrorIs(t, err, domain.ErrProductNotFound)
}
