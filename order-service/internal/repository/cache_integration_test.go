package repository_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"order-service/internal/cache"
	"order-service/internal/domain"
	"order-service/internal/repository"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// countingStorage — «база» со счётчиком вызовов и управляемой задержкой.
type countingStorage struct {
	calls atomic.Int64
	delay time.Duration
	stock atomic.Int64
}

func newCounting(stock int) *countingStorage {
	s := &countingStorage{}
	s.stock.Store(int64(stock))
	return s
}

func (s *countingStorage) GetProductByID(_ context.Context, id int) (*domain.Product, error) {
	s.calls.Add(1)
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	return &domain.Product{ID: id, Name: "P", Price: decimal.NewFromInt(100), Stock: int(s.stock.Load())}, nil
}
func (s *countingStorage) GetProducts(context.Context) ([]domain.Product, error) { return nil, nil }
func (s *countingStorage) InvalidateByID(context.Context, int) error             { return nil }

func newL2(base repository.ProductStorage) *repository.CachedProductRepo {
	return repository.NewCachedProductRepo(base, cache.New(testRedisAddr), zap.NewNop())
}

func redisHas(t *testing.T, id int) bool {
	t.Helper()
	n, err := testRedis.Exists(context.Background(), "product:"+itoa(id)).Result()
	require.NoError(t, err)
	return n == 1
}

func itoa(i int) string { return decimal.NewFromInt(int64(i)).String() }

var ctx = context.Background()

// ---------- уровни ----------

func TestCache_L1Hit_NeitherRedisNorDB(t *testing.T) {
	resetDB(t)
	base := newCounting(5)
	l2 := newL2(base)
	l1 := repository.NewL1ProductRepo(l2)

	_, err := l1.GetProductByID(ctx, 1)
	require.NoError(t, err)
	require.NoError(t, testRedis.FlushAll(ctx).Err()) // даже если Redis пуст

	_, err = l1.GetProductByID(ctx, 1)
	require.NoError(t, err)

	assert.EqualValues(t, 1, base.calls.Load(), "второй вызов не дошёл до базы")
	assert.False(t, redisHas(t, 1), "и не дошёл до Redis: иначе L2 положил бы ключ заново")
}

func TestCache_L2Hit_L1Empty_DBNotCalled(t *testing.T) {
	resetDB(t)
	base := newCounting(5)
	l2 := newL2(base)

	_, err := l2.GetProductByID(ctx, 1) // греем только Redis
	require.NoError(t, err)
	require.True(t, redisHas(t, 1))

	l1 := repository.NewL1ProductRepo(l2) // пустой L1
	p, err := l1.GetProductByID(ctx, 1)
	require.NoError(t, err)

	assert.Equal(t, 5, p.Stock)
	assert.EqualValues(t, 1, base.calls.Load(), "база не вызывалась — ответил Redis")
}

func TestCache_MissEverywhere_FillsBothLevels(t *testing.T) {
	resetDB(t)
	base := newCounting(5)
	l1 := repository.NewL1ProductRepo(newL2(base))

	_, err := l1.GetProductByID(ctx, 1)
	require.NoError(t, err)

	assert.EqualValues(t, 1, base.calls.Load())
	assert.True(t, redisHas(t, 1), "L2 заполнен")

	require.NoError(t, testRedis.FlushAll(ctx).Err())
	_, err = l1.GetProductByID(ctx, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 1, base.calls.Load(), "L1 заполнен")
}

func TestCache_L1TTL_Expires(t *testing.T) {
	resetDB(t)
	base := newCounting(5)
	// Redis не нужен: L1 поверх счётчика напрямую
	l1 := repository.NewL1ProductRepoWithConfig(base, 100, 50*time.Millisecond)

	_, _ = l1.GetProductByID(ctx, 1)
	_, _ = l1.GetProductByID(ctx, 1)
	require.EqualValues(t, 1, base.calls.Load(), "до истечения — из L1")

	require.Eventually(t, func() bool {
		_, _ = l1.GetProductByID(ctx, 1)
		return base.calls.Load() == 2
	}, time.Second, 10*time.Millisecond, "после TTL значение ушло из L1")
}

func TestCache_L1Eviction_OldKeysGone(t *testing.T) {
	resetDB(t)
	base := newCounting(5)
	l1 := repository.NewL1ProductRepo(base) // прод-размер 100

	for id := 1; id <= 101; id++ {
		_, _ = l1.GetProductByID(ctx, id)
	}
	require.EqualValues(t, 101, base.calls.Load())

	_, _ = l1.GetProductByID(ctx, 101) // самый свежий — в кеше
	assert.EqualValues(t, 101, base.calls.Load())

	_, _ = l1.GetProductByID(ctx, 1) // самый старый — вытеснен
	assert.EqualValues(t, 102, base.calls.Load(), "ключ 1 вытеснен при 101-м")
}

// ---------- инвалидация через прод-сборку ----------

// Ровно баг с мержа: если OrderRepo инвалидирует L2, L1 отдаёт старый остаток.
func TestWiring_CancelInvalidatesBothLevels(t *testing.T) {
	resetDB(t)
	pid := seedProduct(t, "A", 100, 10)
	orderRepo, l1 := repository.NewRepositories(testPool, cache.New(testRedisAddr), zap.NewNop())

	id := createOrder(t, orderRepo, domain.OrderItem{ProductID: pid, Quantity: 4})

	p, err := l1.GetProductByID(ctx, pid) // кладём в L1 и L2
	require.NoError(t, err)
	require.Equal(t, 6, p.Stock)

	_, err = orderRepo.CancelOrder(ctx, id)
	require.NoError(t, err)

	p, err = l1.GetProductByID(ctx, pid)
	require.NoError(t, err)
	assert.Equal(t, 10, p.Stock, "после отмены цепочка отдаёт свежий остаток, а не старые 6 из L1")
}

// ---------- singleflight ----------

func TestSingleflight_100Goroutines_OneDBCall(t *testing.T) {
	resetDB(t)
	base := newCounting(5)
	base.delay = 50 * time.Millisecond // все успевают встать в очередь к лидеру
	l2 := newL2(base)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := l2.GetProductByID(ctx, 1)
			assert.NoError(t, err)
		}()
	}
	close(start)
	wg.Wait()

	assert.EqualValues(t, 1, base.calls.Load(), "100 горутин — один поход в базу")
	assert.EqualValues(t, 1, l2.DBCalls())
}

func TestSingleflight_EachGoroutineGetsOwnCopy(t *testing.T) {
	resetDB(t)
	base := newCounting(5)
	base.delay = 50 * time.Millisecond
	l2 := newL2(base)

	const n = 50
	results := make([]*domain.Product, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			p, err := l2.GetProductByID(ctx, 1)
			assert.NoError(t, err)
			results[i] = p
		}(i)
	}
	close(start)
	wg.Wait()
	require.EqualValues(t, 1, base.calls.Load(), "все пришли через одного лидера")

	seen := map[*domain.Product]bool{}
	for _, p := range results {
		assert.False(t, seen[p], "две горутины получили один указатель")
		seen[p] = true
	}

	results[0].Stock = -999
	for _, p := range results[1:] {
		assert.Equal(t, 5, p.Stock, "изменение в одной горутине не видно в другой")
	}
}
