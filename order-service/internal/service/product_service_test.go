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

	svc.GetProductByID(ctx, 1)

	product, err := svc.GetProductByID(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "MacBook Pro", product.Name)
}

func TestInvalidateCache(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	cache := newMockCache()
	svc := NewProductService(repo, cache, zap.NewNop())

	svc.GetProductByID(ctx, 1)
	err := svc.InvalidateCache(ctx, 1)
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
