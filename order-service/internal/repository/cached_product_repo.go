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

// leaderTimeout — предел для похода лидера singleflight в базу.
// Меньше дедлайна запроса (10s): лидер работает на Background и не должен
// держать соединение из пула дольше, чем кто-либо готов ждать ответ.
const leaderTimeout = 3 * time.Second

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
	if errors.Is(err, cache.ErrUnavailable) {
		// предохранитель разомкнут: в Redis не ходили вовсе, штатный режим — без warn
		metrics.CacheErrors.WithLabelValues("l2", "skipped").Inc()
	} else if !errors.Is(err, cache.ErrCacheMiss) {
		// Redis недоступен — деградируем в базу. Ошибку не прячем:
		// лог и метрика нужны, иначе падение кеша станет незаметным.
		r.logger.Warn("redis unavailable, falling back to db", zap.String("key", key), zap.Error(err))
		metrics.CacheErrors.WithLabelValues("l2", "get").Inc()
	}

	r.logger.Debug("cache miss", zap.String("key", key))
	metrics.CacheMisses.WithLabelValues("l2").Inc()

	// Под одним ключом в БД идёт ровно один запрос, остальные ждут его результат.
	//
	// Лидер работает на context.Background(): если он отвалится по таймауту
	// своего клиента, ведомые не должны остаться без ответа. Но «навсегда»
	// не бывает — у лидера свой предел leaderTimeout.
	//
	// DoChan вместо Do: Do не смотрит на контекст ждущего, и клиент висел бы,
	// пока лидер не закончит (при заблокированной таблице — десятки секунд).
	// Через select каждый ждущий уходит по своему дедлайну, лидер доделывает
	// работу и кладёт результат в кеш для следующих.
	ch := r.group.DoChan(key, func() (interface{}, error) {
		dbCtx, cancel := context.WithTimeout(context.Background(), leaderTimeout)
		defer cancel()

		calls := atomic.AddInt64(&r.dbCalls, 1)
		r.logger.Info("db call", zap.Int64("total", calls))
		product, err := r.repo.GetProductByID(dbCtx, id)
		if err != nil {
			return nil, err
		}
		if err := r.cache.Set(dbCtx, key, product, productTTL); err != nil && !errors.Is(err, cache.ErrUnavailable) {
			r.logger.Warn("cache set failed", zap.String("key", key), zap.Error(err))
			metrics.CacheErrors.WithLabelValues("l2", "set").Inc()
		}
		return product, nil
	})

	var val interface{}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		val = res.Val
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
		// Глотаем, но это самое дорогое место: если Redis жив, а Delete не прошёл,
		// старое значение отдаётся до истечения productTTL.
		r.logger.Error("cache delete failed, stale value may be served until TTL",
			zap.String("key", key), zap.Error(err))
		metrics.CacheErrors.WithLabelValues("l2", "delete").Inc()
	}
	r.group.Forget(key)
	return nil
}
