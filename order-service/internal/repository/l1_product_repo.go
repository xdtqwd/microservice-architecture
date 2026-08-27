package repository

import (
	"context"
	"fmt"
	"order-service/internal/domain"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

const (
	l1MaxSize = 100
	l1TTL     = 10 * time.Second
)

type L1ProductRepo struct {
	repo  *CachedProductRepo
	cache *lru.LRU[string, *domain.Product]
}

func NewL1ProductRepo(repo *CachedProductRepo) *L1ProductRepo {
	cache := lru.NewLRU[string, *domain.Product](l1MaxSize, nil, l1TTL)
	return &L1ProductRepo{repo: repo, cache: cache}
}

func (r *L1ProductRepo) GetProducts(ctx context.Context) ([]domain.Product, error) {
	return r.repo.GetProducts(ctx)
}
func (r *L1ProductRepo) GetProductByID(ctx context.Context, id int) (*domain.Product, error) {
	key := fmt.Sprintf("product:%d", id)

	if p, ok := r.cache.Get(key); ok {
		return p, nil
	}

	p, err := r.repo.GetProductByID(ctx, id)
	if err != nil {
		return nil, err
	}

	r.cache.Add(key, p)
	return p, nil
}

func (r *L1ProductRepo) InvalidateByID(ctx context.Context, id int) error {
	key := fmt.Sprintf("product:%d", id)
	r.cache.Remove(key)
	return r.repo.InvalidateByID(ctx, id)
}
