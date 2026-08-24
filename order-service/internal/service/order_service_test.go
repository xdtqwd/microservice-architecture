package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"order-service/internal/domain"
)

func TestCreateOrder_Success(t *testing.T) {

	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo)

	items := []domain.CreateOrderItem{
		{ProductID: 1, Quantity: 2},
	}

	id, err := svc.CreateOrder(ctx, items)
	assert.NoError(t, err)
	assert.Equal(t, 1, id)
}

func TestCancelOrder_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo)

	items := []domain.CreateOrderItem{
		{ProductID: 1, Quantity: 2},
	}
	id, _ := svc.CreateOrder(ctx, items)

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
	svc := NewOrderService(repo)

	items := []domain.CreateOrderItem{
		{ProductID: 1, Quantity: 2},
	}

	_, err := svc.CreateOrder(ctx, items)
	assert.NoError(t, err)

	_, err = svc.CreateOrder(ctx, items)
	assert.NoError(t, err)

	orders, err := svc.GetOrders(ctx, 10, 0)
	assert.NoError(t, err)
	assert.Len(t, orders, 2)
}
func TestCreateOrder_InvalidQuantity(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo)

	items := []domain.CreateOrderItem{
		{ProductID: 1, Quantity: -1},
	}

	_, err := svc.CreateOrder(ctx, items)
	assert.Error(t, err)
}
func TestCreateOrder_EmptyItems(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo)

	_, err := svc.CreateOrder(ctx, []domain.CreateOrderItem{})
	assert.Error(t, err)
}

func TestGetOrderByID_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo)

	order, err := svc.GetOrderByID(ctx, 999)
	assert.NoError(t, err)
	assert.Nil(t, order)
}

func TestCancelOrder_AlreadyCancelled(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo)

	items := []domain.CreateOrderItem{
		{ProductID: 1, Quantity: 2},
	}
	id, _ := svc.CreateOrder(ctx, items)

	_, err := svc.CancelOrder(ctx, id)
	assert.NoError(t, err)

	_, err = svc.CancelOrder(ctx, id)
	assert.Error(t, err)
}

func TestGetOrders_Pagination(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo)

	items := []domain.CreateOrderItem{{ProductID: 1, Quantity: 1}}
	for i := 0; i < 5; i++ {
		_, err := svc.CreateOrder(ctx, items)
		assert.NoError(t, err)
	}

	orders, err := svc.GetOrders(ctx, 10000, 0)
	assert.NoError(t, err)
	assert.LessOrEqual(t, len(orders), 100)

	orders, err = svc.GetOrders(ctx, -1, 0)
	assert.NoError(t, err)
	assert.NotNil(t, orders)

	orders, err = svc.GetOrders(ctx, 2, 0)
	assert.NoError(t, err)
	assert.Len(t, orders, 2)

	orders, err = svc.GetOrders(ctx, 2, 2)
	assert.NoError(t, err)
	assert.Len(t, orders, 2)
}
