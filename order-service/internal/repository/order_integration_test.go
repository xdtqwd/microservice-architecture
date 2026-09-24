package repository_test

import (
	"context"
	"testing"

	"order-service/internal/domain"
	"order-service/internal/repository"
	"order-service/internal/txm"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type noopInvalidator struct{}

func (noopInvalidator) InvalidateByID(context.Context, int) error { return nil }

func newOrderRepo() *repository.OrderRepo {
	return repository.NewOrderRepo(testPool, noopInvalidator{}, zap.NewNop())
}

// createInTx вызывает CreateOrder так же, как сервис: внутри txm.Do.
// Без внешней транзакции репозиторий работает на пуле и не атомарен.
func createInTx(repo *repository.OrderRepo, items []domain.OrderItem, key string) (int, bool, error) {
	var id int
	var exists bool
	err := txm.New(testPool).Do(context.Background(), func(ctx context.Context) error {
		var e error
		id, exists, e = repo.CreateOrder(ctx, items, key)
		return e
	})
	return id, exists, err
}

func createOrder(t *testing.T, repo *repository.OrderRepo, items ...domain.OrderItem) int {
	t.Helper()
	id, _, err := createInTx(repo, items, "")
	require.NoError(t, err)
	return id
}

// ---------- CreateOrder ----------

func TestCreateOrder_DeductsExactStock(t *testing.T) {
	resetDB(t)
	a := seedProduct(t, "A", 100, 10)
	b := seedProduct(t, "B", 200, 5)

	createOrder(t, newOrderRepo(),
		domain.OrderItem{ProductID: a, Quantity: 3},
		domain.OrderItem{ProductID: b, Quantity: 2})

	assert.Equal(t, 7, stockOf(t, a))
	assert.Equal(t, 3, stockOf(t, b))
	assert.Equal(t, 1, countRows(t, "orders"))
	assert.Equal(t, 2, countRows(t, "order_items"))
}

func TestCreateOrder_PriceFromDBNotFromClient(t *testing.T) {
	resetDB(t)
	pid := seedProduct(t, "A", 1500, 10)
	repo := newOrderRepo()

	id := createOrder(t, repo, domain.OrderItem{ProductID: pid, Quantity: 1, Price: decimal.NewFromInt(1)})

	order, err := repo.GetOrderByID(context.Background(), id)
	require.NoError(t, err)
	require.Len(t, order.Items, 1)
	assert.True(t, decimal.NewFromInt(1500).Equal(order.Items[0].Price),
		"цена должна браться из products, получили %s", order.Items[0].Price)
}

func TestCreateOrder_InsufficientStock_NothingChanges(t *testing.T) {
	resetDB(t)
	pid := seedProduct(t, "A", 100, 1)

	_, _, err := createInTx(newOrderRepo(), []domain.OrderItem{{ProductID: pid, Quantity: 2}}, "")

	assert.ErrorIs(t, err, domain.ErrInsufficientStock)
	assert.Equal(t, 1, stockOf(t, pid))
	assert.Equal(t, 0, countRows(t, "orders"), "заказ не должен остаться после отката")
}

func TestCreateOrder_UnknownProduct_NothingChanges(t *testing.T) {
	resetDB(t)
	a := seedProduct(t, "A", 100, 10)

	_, _, err := createInTx(newOrderRepo(),
		[]domain.OrderItem{{ProductID: a, Quantity: 1}, {ProductID: 99999, Quantity: 1}}, "")

	assert.ErrorIs(t, err, domain.ErrProductNotFound)
	assert.Equal(t, 10, stockOf(t, a), "списание по первому товару должно откатиться")
	assert.Equal(t, 0, countRows(t, "orders"))
}

func TestCreateOrder_IdempotencyKey_OneOrderOneDeduction(t *testing.T) {
	resetDB(t)
	pid := seedProduct(t, "A", 100, 10)
	repo := newOrderRepo()
	items := []domain.OrderItem{{ProductID: pid, Quantity: 2}}

	id1, exists1, err := createInTx(repo, items, "key-1")
	require.NoError(t, err)
	id2, exists2, err := createInTx(repo, items, "key-1")
	require.NoError(t, err)

	assert.False(t, exists1)
	assert.True(t, exists2)
	assert.Equal(t, id1, id2)
	assert.Equal(t, 8, stockOf(t, pid), "повтор с тем же ключом не списывает второй раз")
	assert.Equal(t, 1, countRows(t, "orders"))
}

// ---------- GetOrderByID ----------

func TestGetOrderByID_NotFound(t *testing.T) {
	resetDB(t)
	order, err := newOrderRepo().GetOrderByID(context.Background(), 12345)
	assert.ErrorIs(t, err, domain.ErrOrderNotFound)
	assert.Nil(t, order)
}

func TestGetOrderByID_ReturnsItems(t *testing.T) {
	resetDB(t)
	a := seedProduct(t, "A", 100, 10)
	b := seedProduct(t, "B", 200, 10)
	repo := newOrderRepo()
	id := createOrder(t, repo,
		domain.OrderItem{ProductID: a, Quantity: 1},
		domain.OrderItem{ProductID: b, Quantity: 3})

	order, err := repo.GetOrderByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, id, order.ID)
	assert.Equal(t, "pending", order.Status)
	assert.Len(t, order.Items, 2)
}

// ---------- GetOrders ----------

func collectAll(t *testing.T, repo *repository.OrderRepo, limit int) []int {
	t.Helper()
	var ids []int
	var cursor *domain.OrderCursor
	for page := 0; page < 20; page++ {
		orders, next, err := repo.GetOrders(context.Background(), limit, cursor)
		require.NoError(t, err)
		for _, o := range orders {
			ids = append(ids, o.ID)
		}
		if next == nil || len(orders) == 0 {
			break
		}
		cursor = next
	}
	return ids
}

func TestGetOrders_KeysetThreePages_NoLossNoDuplicates(t *testing.T) {
	resetDB(t)
	pid := seedProduct(t, "A", 100, 100)
	repo := newOrderRepo()
	want := map[int]bool{}
	for i := 0; i < 9; i++ {
		want[createOrder(t, repo, domain.OrderItem{ProductID: pid, Quantity: 1})] = true
	}

	ids := collectAll(t, repo, 3)

	seen := map[int]bool{}
	for _, id := range ids {
		assert.False(t, seen[id], "заказ %d задвоен", id)
		seen[id] = true
	}
	assert.Equal(t, want, seen, "каждый заказ ровно один раз")
	for i := 1; i < len(ids); i++ {
		assert.Greater(t, ids[i-1], ids[i], "порядок по id DESC")
	}
}

// Удаление записи с уже прочитанной страницы между запросами:
// с OFFSET следующая страница «съехала» бы и один заказ потерялся.
// Keyset продолжает от курсора и ничего не теряет.
func TestGetOrders_DeleteBetweenPages_NothingLost(t *testing.T) {
	resetDB(t)
	pid := seedProduct(t, "A", 100, 100)
	repo := newOrderRepo()
	for i := 0; i < 8; i++ {
		createOrder(t, repo, domain.OrderItem{ProductID: pid, Quantity: 1})
	}
	ctx := context.Background()

	page1, cursor, err := repo.GetOrders(ctx, 4, nil)
	require.NoError(t, err)
	require.Len(t, page1, 4)
	require.NotNil(t, cursor)

	// удаляем заказ с первой страницы
	deleted := page1[0].ID
	_, err = testPool.Exec(ctx, "DELETE FROM order_items WHERE order_id = $1", deleted)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, "DELETE FROM orders WHERE id = $1", deleted)
	require.NoError(t, err)

	page2, _, err := repo.GetOrders(ctx, 4, cursor)
	require.NoError(t, err)

	// вторая страница — ровно следующие 4 заказа после последнего из первой
	require.Len(t, page2, 4)
	assert.Equal(t, page1[3].ID-1, page2[0].ID, "ни один заказ между страницами не пропущен")
}

// ---------- CancelOrder ----------

func TestCancelOrder_RestoresStockForAllItems(t *testing.T) {
	resetDB(t)
	a := seedProduct(t, "A", 100, 10)
	b := seedProduct(t, "B", 200, 5)
	repo := newOrderRepo()
	id := createOrder(t, repo,
		domain.OrderItem{ProductID: a, Quantity: 4},
		domain.OrderItem{ProductID: b, Quantity: 5})
	require.Equal(t, 6, stockOf(t, a))
	require.Equal(t, 0, stockOf(t, b))

	_, err := repo.CancelOrder(context.Background(), id)
	require.NoError(t, err)

	assert.Equal(t, 10, stockOf(t, a), "остаток A вернулся")
	assert.Equal(t, 5, stockOf(t, b), "остаток B вернулся")
}

func TestCancelOrder_Twice_ErrAlreadyCancelled_StockOnce(t *testing.T) {
	resetDB(t)
	pid := seedProduct(t, "A", 100, 10)
	repo := newOrderRepo()
	id := createOrder(t, repo, domain.OrderItem{ProductID: pid, Quantity: 3})

	_, err := repo.CancelOrder(context.Background(), id)
	require.NoError(t, err)
	_, err = repo.CancelOrder(context.Background(), id)

	assert.ErrorIs(t, err, domain.ErrOrderAlreadyCancelled)
	assert.Equal(t, 10, stockOf(t, pid), "остаток вернулся ровно один раз")
}

func TestCancelOrder_FromShippedOrDelivered_Forbidden(t *testing.T) {
	for _, status := range []string{"shipped", "delivered"} {
		t.Run(status, func(t *testing.T) {
			resetDB(t)
			pid := seedProduct(t, "A", 100, 10)
			repo := newOrderRepo()
			id := createOrder(t, repo, domain.OrderItem{ProductID: pid, Quantity: 2})
			setStatus(t, id, status)

			_, err := repo.CancelOrder(context.Background(), id)

			assert.ErrorIs(t, err, domain.ErrInvalidStatusTransition)
			assert.Equal(t, 8, stockOf(t, pid), "остаток не возвращается")
		})
	}
}

func TestCancelOrder_NotFound(t *testing.T) {
	resetDB(t)
	_, err := newOrderRepo().CancelOrder(context.Background(), 12345)
	assert.ErrorIs(t, err, domain.ErrOrderNotFound)
}
