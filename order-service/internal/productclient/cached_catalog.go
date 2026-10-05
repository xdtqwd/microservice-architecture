package productclient

import (
	"context"
	"time"

	"order-service/internal/metrics"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/shopspring/decimal"
)

type PriceSource interface {
	Prices(ctx context.Context, ids []int) (map[int]decimal.Decimal, error)
}

// CachedCatalog — кеш цен поверх любого источника, та же схема декораторов,
// что L1 поверх Redis. Соседа спрашиваем только про товары, которых нет в кеше.
//
// Цена: новая цена из product-service доходит до заказов с задержкой до ttl.
// Отсутствие товара не кешируем — новый товар должен продаваться сразу.
type CachedCatalog struct {
	next  PriceSource
	cache *lru.LRU[int, decimal.Decimal]
}

func NewCachedCatalog(next PriceSource, size int, ttl time.Duration) *CachedCatalog {
	return &CachedCatalog{next: next, cache: lru.NewLRU[int, decimal.Decimal](size, nil, ttl)}
}

func (c *CachedCatalog) Prices(ctx context.Context, ids []int) (map[int]decimal.Decimal, error) {
	out := make(map[int]decimal.Decimal, len(ids))
	var missing []int
	for _, id := range ids {
		if _, done := out[id]; done {
			continue
		}
		if v, ok := c.cache.Get(id); ok {
			out[id] = v
			metrics.CacheHits.WithLabelValues("catalog").Inc()
			continue
		}
		missing = append(missing, id)
	}
	if len(missing) == 0 {
		return out, nil
	}

	metrics.CacheMisses.WithLabelValues("catalog").Add(float64(len(missing)))
	fetched, err := c.next.Prices(ctx, missing)
	if err != nil {
		return nil, err
	}
	for id, v := range fetched {
		c.cache.Add(id, v)
		out[id] = v
	}
	return out, nil
}
