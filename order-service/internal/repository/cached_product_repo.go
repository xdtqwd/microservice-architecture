package repository

import (
	"context"
	"fmt"
	"order-service/internal/cache"
	"order-service/internal/domain"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
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
	group  singleflight.Group
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
	if err := r.cache.Get(ctx, key, &p); err == nil {
		r.logger.Debug("cache hit", zap.String("key", key))
		return &p, nil
	}

	r.logger.Debug("cache miss", zap.String("key", key))

	val, err, _ := r.group.Do(key, func() (interface{}, error) {
		product, err := r.repo.GetProductByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := r.cache.Set(ctx, key, product, productTTL); err != nil {
			r.logger.Error("cache set error", zap.Error(err))
		}
		return product, nil
	})
	if err != nil {
		return nil, err
	}

	return val.(*domain.Product), nil
}

func (r *CachedProductRepo) InvalidateByID(ctx context.Context, id int) error {
	key := fmt.Sprintf("product:%d", id)
	if err := r.cache.Delete(ctx, key); err != nil {
		r.logger.Error("cache delete error", zap.Error(err))
	}
	return nil
}
