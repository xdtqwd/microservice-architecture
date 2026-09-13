package service

import (
	"context"
	"go.uber.org/zap"
	"testing"

	"github.com/stretchr/testify/assert"
	"order-service/internal/domain"
	"github.com/shopspring/decimal"
)

func TestCreateOrder_Success(t *testing.T) {

	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo, nil, zap.NewNop(), nil)

	items := []domain.CreateOrderItem{
		{ProductID: 1, Quantity: 2},
	}

	id, _, err := svc.CreateOrder(ctx, items, "")
	assert.NoError(t, err)
	assert.Equal(t, 1, id)
}

func TestCancelOrder_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo, nil, zap.NewNop(), nil)

	items := []domain.CreateOrderItem{
		{ProductID: 1, Quantity: 2},
	}
	id, _, _ := svc.CreateOrder(ctx, items, "")

	cancelledID, err := svc.CancelOrder(ctx, id)
	assert.NoError(t, err)
	assert.Equal(t, id, cancelledID)

	order, err := svc.GetOrderByID(ctx, id)
	assert.NoError(t, err)
	assert.Equal(t, "cancelled", order.Status)
}

func TestGetOrders_ReturnsAll(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo, nil, zap.NewNop(), nil)

	items := []domain.CreateOrderItem{
		{ProductID: 1, Quantity: 2},
	}

	_, _, err := svc.CreateOrder(ctx, items, "")
	assert.NoError(t, err)

	_, _, err = svc.CreateOrder(ctx, items, "")
	assert.NoError(t, err)

	orders, _, err := svc.GetOrders(ctx, 10, nil)
	assert.NoError(t, err)
	assert.Len(t, orders, 2)
}
func TestCreateOrder_InvalidQuantity(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo, nil, zap.NewNop(), nil)

	items := []domain.CreateOrderItem{
		{ProductID: 1, Quantity: -1},
	}

	_, _, err := svc.CreateOrder(ctx, items, "")
	assert.Error(t, err)
}
func TestCreateOrder_EmptyItems(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo, nil, zap.NewNop(), nil)

	_, _, err := svc.CreateOrder(ctx, []domain.CreateOrderItem{}, "")
	assert.Error(t, err)
}

func TestGetOrderByID_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo, nil, zap.NewNop(), nil)

	order, err := svc.GetOrderByID(ctx, 999)
	assert.ErrorIs(t, err, domain.ErrOrderNotFound)
	assert.Nil(t, order)
}

func TestCancelOrder_AlreadyCancelled(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo, nil, zap.NewNop(), nil)

	items := []domain.CreateOrderItem{
		{ProductID: 1, Quantity: 2},
	}
	id, _, _ := svc.CreateOrder(ctx, items, "")

	_, err := svc.CancelOrder(ctx, id)
	assert.NoError(t, err)

	_, err = svc.CancelOrder(ctx, id)
	assert.Error(t, err)
}

func TestGetOrders_Pagination(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo, nil, zap.NewNop(), nil)

	items := []domain.CreateOrderItem{{ProductID: 1, Quantity: 1}}
	for i := 0; i < 5; i++ {
		_, _, err := svc.CreateOrder(ctx, items, "")
		assert.NoError(t, err)
	}

	orders, _, err := svc.GetOrders(ctx, 10000, nil)
	assert.NoError(t, err)
	assert.LessOrEqual(t, len(orders), 100)

	orders, _, err = svc.GetOrders(ctx, -1, nil)
	assert.NoError(t, err)
	assert.NotNil(t, orders)

	orders, _, err = svc.GetOrders(ctx, 2, nil)
	assert.NoError(t, err)
	assert.Len(t, orders, 2)

	orders, _, err = svc.GetOrders(ctx, 2, &domain.OrderCursor{AfterID: 2})
	assert.NoError(t, err)
	assert.Len(t, orders, 2)
}

func TestCreateOrder_IdempotencyParallel(t *testing.T) {
	repo := newMockRepo()
	repo.products = append(repo.products, domain.Product{
		ID:    1,
		Name:  "Test",
		Price: decimal.NewFromFloat(10.0),
		Stock: 100,
	})
	svc := NewOrderService(repo, nil, zap.NewNop(), nil)
	ctx := context.Background()

	items := []domain.CreateOrderItem{{ProductID: 1, Quantity: 1}}
	key := "parallel-test-key"

	type result struct {
		id  int
		err error
	}
	results := make(chan result, 10)

	for i := 0; i < 10; i++ {
		go func() {
			id, _, err := svc.CreateOrder(ctx, items, key)
			results <- result{id, err}
		}()
	}

	ids := make(map[int]bool)
	for i := 0; i < 10; i++ {
		r := <-results
		assert.NoError(t, r.err)
		if r.id > 0 {
			ids[r.id] = true
		}
	}
	assert.Equal(t, 1, len(ids), "должен создаться ровно один заказ")
}
