package productclient

import (
	"context"
	"errors"
	"time"

	"order-service/internal/domain"
	"order-service/internal/metrics"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/shopspring/decimal"
)

type PriceSource interface {
	Prices(ctx context.Context, ids []int) (map[int]decimal.Decimal, error)
}

type cachedPrice struct {
	price decimal.Decimal
	at    time.Time
}

// CachedCatalog — кеш цен поверх любого источника.
//
//   - моложе ttl — свежая цена, к соседу не ходим;
//   - старше ttl — спрашиваем соседа;
//   - сосед недоступен, а цена моложе maxStale — отдаём устаревшую.
//
// Устаревшая цена — осознанный размен: заказы продолжают идти, но по цене
// не старше maxStale. Остаток при этом списывается атомарно в нашей базе,
// так что риск — только в цене. Отсутствие товара и ошибки не кешируем.
type CachedCatalog struct {
	next     PriceSource
	cache    *lru.Cache[int, cachedPrice]
	ttl      time.Duration
	maxStale time.Duration
	now      func() time.Time
}

func NewCachedCatalog(next PriceSource, size int, ttl time.Duration) *CachedCatalog {
	cache, _ := lru.New[int, cachedPrice](size)
	return &CachedCatalog{next: next, cache: cache, ttl: ttl, now: time.Now}
}

// WithMaxStale разрешает отдавать цену не старше d, если каталог недоступен.
func (c *CachedCatalog) WithMaxStale(d time.Duration) *CachedCatalog {
	c.maxStale = d
	return c
}

func (c *CachedCatalog) Prices(ctx context.Context, ids []int) (map[int]decimal.Decimal, error) {
	now := c.now()
	out := make(map[int]decimal.Decimal, len(ids))
	stale := map[int]decimal.Decimal{}
	var missing []int
	seen := map[int]bool{}

	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if e, ok := c.cache.Get(id); ok {
			age := now.Sub(e.at)
			if age < c.ttl {
				out[id] = e.price
				metrics.CacheHits.WithLabelValues("catalog").Inc()
				continue
			}
			if c.maxStale > 0 && age < c.maxStale {
				stale[id] = e.price
			}
		}
		missing = append(missing, id)
	}
	if len(missing) == 0 {
		return out, nil
	}

	metrics.CacheMisses.WithLabelValues("catalog").Add(float64(len(missing)))
	fetched, err := c.next.Prices(ctx, missing)
	if err != nil {
		// сосед недоступен, но для всех недостающих товаров есть цена не старше
		// maxStale — продаём по ней. Если хоть одного нет — честный отказ.
		if errors.Is(err, domain.ErrCatalogUnavailable) && len(stale) == len(missing) {
			for id, p := range stale {
				out[id] = p
			}
			metrics.CatalogStaleServed.Add(float64(len(stale)))
			return out, nil
		}
		return nil, err
	}
	for id, v := range fetched {
		c.cache.Add(id, cachedPrice{price: v, at: now})
		out[id] = v
	}
	return out, nil
}
