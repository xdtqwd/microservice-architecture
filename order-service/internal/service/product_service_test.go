package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestGetProductByID_CacheMiss(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	cache := newMockCache()
	svc := NewProductService(repo, cache, zap.NewNop())

	product, err := svc.GetProductByID(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "MacBook Pro", product.Name)
}

func TestGetProductByID_CacheHit(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	cache := newMockCache()
	svc := NewProductService(repo, cache, zap.NewNop())

	_, err := svc.GetProductByID(ctx, 1)
	assert.NoError(t, err)

	product, err := svc.GetProductByID(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "MacBook Pro", product.Name)
}

func TestInvalidateCache(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	cache := newMockCache()
	svc := NewProductService(repo, cache, zap.NewNop())

	_, err := svc.GetProductByID(ctx, 1)
	assert.NoError(t, err)
	err = svc.InvalidateCache(ctx, 1)
	assert.NoError(t, err)
}
func TestGetProducts(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	cache := newMockCache()
	svc := NewProductService(repo, cache, zap.NewNop())

	products, err := svc.GetProducts(ctx)
	assert.NoError(t, err)
	assert.Len(t, products, 2)
}
func TestGetOrders_LimitCappedToMax(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewOrderService(repo)

	items := []CreateOrderItem{{ProductID: 1, Quantity: 1}}
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
