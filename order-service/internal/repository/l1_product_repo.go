package repository

import (
	"context"
	"fmt"
	"order-service/internal/domain"
	"time"

	"order-service/internal/metrics"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

const (
	l1MaxSize = 100
	l1TTL     = 10 * time.Second
)

func productKey(id int) string {
	return fmt.Sprintf("product:%d", id)
}

type StorageWithInvalidation interface {
	ProductStorage
	InvalidateByID(ctx context.Context, id int) error
}

type L1ProductRepo struct {
	repo  StorageWithInvalidation
	cache *lru.LRU[string, domain.Product]
}

func NewL1ProductRepo(repo StorageWithInvalidation) *L1ProductRepo {
	return NewL1ProductRepoWithConfig(repo, l1MaxSize, l1TTL)
}

// NewL1ProductRepoWithConfig позволяет задать размер и TTL — нужно тестам,
// чтобы проверять истечение за миллисекунды, а не ждать 10 секунд.
func NewL1ProductRepoWithConfig(repo StorageWithInvalidation, size int, ttl time.Duration) *L1ProductRepo {
	cache := lru.NewLRU[string, domain.Product](size, nil, ttl)
	return &L1ProductRepo{repo: repo, cache: cache}
}

func (r *L1ProductRepo) GetProducts(ctx context.Context) ([]domain.Product, error) {
	return r.repo.GetProducts(ctx)
}

func (r *L1ProductRepo) GetProductByID(ctx context.Context, id int) (*domain.Product, error) {
	key := productKey(id)

	if p, ok := r.cache.Get(key); ok {
		metrics.CacheHits.WithLabelValues("l1").Inc()
		copy := p
		return &copy, nil
	}

	p, err := r.repo.GetProductByID(ctx, id)
	if err != nil {
		return nil, err
	}

	metrics.CacheMisses.WithLabelValues("l1").Inc()
	r.cache.Add(key, *p)
	return p, nil
}

func (r *L1ProductRepo) InvalidateByID(ctx context.Context, id int) error {
	r.cache.Remove(productKey(id))
	return r.repo.InvalidateByID(ctx, id)
}
