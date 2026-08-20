package service

import (
	"context"
	"order-service/internal/repository"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateOrder_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo)

	items := []repository.OrderItem{
		{ProductID: 1, Quantity: 2, Price: 150000},
	}

	id, err := svc.CreateOrder(ctx, items)
	assert.NoError(t, err)
	assert.Equal(t, 1, id)
}

func TestCancelOrder_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo)

	items := []repository.OrderItem{{ProductID: 1, Quantity: 1, Price: 150000}}
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

	items := []repository.OrderItem{{ProductID: 1, Quantity: 1, Price: 150000}}
	_, err := svc.CreateOrder(ctx, items)
	assert.NoError(t, err)

	orders, err := svc.GetOrders(ctx)
	assert.NoError(t, err)
	assert.Len(t, orders, 2)
}

func TestCreateOrder_InvalidQuantity(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo)

	items := []repository.OrderItem{
		{ProductID: 1, Quantity: -1, Price: 150000},
	}

	_, err := svc.CreateOrder(ctx, items)
	assert.Error(t, err)
}
func TestCreateOrder_EmptyItems(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo)

	_, err := svc.CreateOrder(ctx, []repository.OrderItem{})
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

	items := []repository.OrderItem{{ProductID: 1, Quantity: 1, Price: 150000}}
	id, _ := svc.CreateOrder(ctx, items)

	_, err := svc.CancelOrder(ctx, id)
	assert.Error(t, err)
}
