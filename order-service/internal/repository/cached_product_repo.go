package repository

import (
	"context"
	"errors"
	"fmt"
	"order-service/internal/cache"
	"order-service/internal/domain"
	"time"

	"go.uber.org/zap"
	"order-service/internal/metrics"
)

const productTTL = 5 * time.Minute

type ProductStorage interface {
	GetProducts(ctx context.Context) ([]domain.Product, error)
	GetProductByID(ctx context.Context, id int) (*domain.Product, error)
}

type CachedProductRepo struct {
	repo   ProductStorage
	cache  *cache.RedisCache
	logger *zap.Logger
}

func NewCachedProductRepo(repo ProductStorage, c *cache.RedisCache, logger *zap.Logger) *CachedProductRepo {
	return &CachedProductRepo{repo: repo, cache: c, logger: logger}
}

func (r *CachedProductRepo) GetProducts(ctx context.Context) ([]domain.Product, error) {
	return r.repo.GetProducts(ctx)
}

func (r *CachedProductRepo) GetProductByID(ctx context.Context, id int) (*domain.Product, error) {
	key := fmt.Sprintf("product:%d", id)

	var p domain.Product
	err := r.cache.Get(ctx, key, &p)
	if err == nil {
		r.logger.Debug("cache hit", zap.String("key", key))
		metrics.CacheHits.WithLabelValues("l2").Inc()
		return &p, nil
	}
	if !errors.Is(err, cache.ErrCacheMiss) {
		r.logger.Error("redis error", zap.Error(err))
		return nil, err
	}

	r.logger.Debug("cache miss", zap.String("key", key))
	metrics.CacheMisses.WithLabelValues("l2").Inc()
	product, err := r.repo.GetProductByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := r.cache.Set(ctx, key, product, productTTL); err != nil {
		r.logger.Error("cache set error", zap.Error(err))
	}
	return product, nil
}

func (r *CachedProductRepo) InvalidateByID(ctx context.Context, id int) error {
	key := fmt.Sprintf("product:%d", id)
	return r.cache.Delete(ctx, key)
}
