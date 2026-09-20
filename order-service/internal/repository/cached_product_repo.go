package repository

import (
	"context"
	"fmt"
	"order-service/internal/cache"
	"order-service/internal/domain"
	"order-service/internal/metrics"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

const productTTL = 5 * time.Minute

type ProductStorage interface {
	GetProducts(ctx context.Context) ([]domain.Product, error)
	GetProductByID(ctx context.Context, id int) (*domain.Product, error)
	InvalidateByID(ctx context.Context, id int) error
}

type CachedProductRepo struct {
	repo      ProductStorage
	cache     *cache.RedisCache
	logger    *zap.Logger
	group     singleflight.Group
	versions  sync.Map // map[int]*int64 — поколение ключа
}

func NewCachedProductRepo(repo ProductStorage, c *cache.RedisCache, logger *zap.Logger) *CachedProductRepo {
	return &CachedProductRepo{repo: repo, cache: c, logger: logger}
}

func (r *CachedProductRepo) generation(id int) *int64 {
	v, _ := r.versions.LoadOrStore(id, new(int64))
	return v.(*int64)
}

func (r *CachedProductRepo) GetProducts(ctx context.Context) ([]domain.Product, error) {
	return r.repo.GetProducts(ctx)
}

func (r *CachedProductRepo) GetProductByID(ctx context.Context, id int) (*domain.Product, error) {
	key := fmt.Sprintf("product:%d", id)

	var p domain.Product
	if err := r.cache.Get(ctx, key, &p); err == nil {
		r.logger.Debug("cache hit", zap.String("key", key))
		metrics.CacheHits.WithLabelValues("l2").Inc()
		cp := p
		return &cp, nil
	}

	r.logger.Debug("cache miss", zap.String("key", key))
	metrics.CacheMisses.WithLabelValues("l2").Inc()

	// запомним поколение до похода в БД
	gen := atomic.LoadInt64(r.generation(id))

	val, err, _ := r.group.Do(key, func() (interface{}, error) {
		product, err := r.repo.GetProductByID(context.Background(), id)
		if err != nil {
			return nil, err
		}
		// проверяем что инвалидации не было пока мы ходили в БД
		if atomic.LoadInt64(r.generation(id)) == gen {
			if err := r.cache.Set(context.Background(), key, product, productTTL); err != nil {
				r.logger.Error("cache set error", zap.Error(err))
			}
		} else {
			r.logger.Debug("skipping cache set: generation changed", zap.String("key", key))
		}
		return product, nil
	})
	if err != nil {
		return nil, err
	}

	cp := *val.(*domain.Product)
	return &cp, nil
}

func (r *CachedProductRepo) InvalidateByID(ctx context.Context, id int) error {
	key := fmt.Sprintf("product:%d", id)
	// увеличиваем поколение ДО удаления из кэша
	// лидер singleflight сверит своё поколение и не запишет устаревшее значение
	atomic.AddInt64(r.generation(id), 1)
	if err := r.cache.Delete(ctx, key); err != nil {
		r.logger.Error("cache delete error", zap.Error(err))
	}
	r.group.Forget(key)
	return r.repo.InvalidateByID(ctx, id)
}
