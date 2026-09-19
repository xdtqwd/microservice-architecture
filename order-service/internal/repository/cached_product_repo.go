package repository

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"order-service/internal/cache"
	"order-service/internal/domain"
	"order-service/internal/metrics"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

const productTTL = 5 * time.Minute

type ProductStorage interface {
	GetProducts(ctx context.Context) ([]domain.Product, error)
	GetProductByID(ctx context.Context, id int) (*domain.Product, error)
}

type CachedProductRepo struct {
	repo    ProductStorage
	cache   *cache.RedisCache
	logger  *zap.Logger
	group   singleflight.Group
	dbCalls int64
}

func NewCachedProductRepo(repo ProductStorage, c *cache.RedisCache, logger *zap.Logger) *CachedProductRepo {
	return &CachedProductRepo{repo: repo, cache: c, logger: logger}
}

func (r *CachedProductRepo) DBCalls() int64 {
	return atomic.LoadInt64(&r.dbCalls)
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

	// под одним ключом в БД идёт ровно один запрос, остальные ждут его результат.
	// внутри context.Background(): если лидер отвалится по таймауту, ведомые
	// не должны остаться без ответа
	val, err, _ := r.group.Do(key, func() (interface{}, error) {
		calls := atomic.AddInt64(&r.dbCalls, 1)
		r.logger.Info("db call", zap.Int64("total", calls))
		product, err := r.repo.GetProductByID(context.Background(), id)
		if err != nil {
			return nil, err
		}
		if err := r.cache.Set(context.Background(), key, product, productTTL); err != nil {
			r.logger.Error("cache set error", zap.Error(err))
		}
		return product, nil
	})
	if err != nil {
		return nil, err
	}

	// копия, иначе все ведомые получат один указатель на общий объект
	cp := *val.(*domain.Product)
	return &cp, nil
}

func (r *CachedProductRepo) InvalidateByID(ctx context.Context, id int) error {
	key := fmt.Sprintf("product:%d", id)
	// порядок важен: сначала Delete, потом Forget.
	// Forget убирает ключ из карты, и новые запросы не ждут лидера,
	// но уже запущенный лидер всё равно запишет прочитанное до сброса.
	// окно гонки остаётся узким — между Do и Set лидера
	if err := r.cache.Delete(ctx, key); err != nil {
		r.logger.Error("cache delete error", zap.Error(err))
	}
	r.group.Forget(key)
	return nil
}
