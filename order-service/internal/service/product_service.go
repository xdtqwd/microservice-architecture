package service

import (
	"context"
	"fmt"
	"order-service/internal/cache"
	"order-service/internal/repository"
	"time"
)

type ProductService struct {
	repo  *repository.Repository
	cache *cache.RedisCache
}

func NewProductService(repo *repository.Repository, cache *cache.RedisCache) *ProductService {
	return &ProductService{repo: repo, cache: cache}
}

func (s *ProductService) GetProducts(ctx context.Context) ([]repository.Product, error) {
	return s.repo.GetProducts(ctx)
}

func (s *ProductService) GetProductByID(ctx context.Context, id int) (*repository.Product, error) {
	key := fmt.Sprintf("product:%d", id)

	var product repository.Product
	err := s.cache.Get(ctx, key, &product)
	if err == nil {
		fmt.Println("cache hit:", key)
		return &product, nil
	}

	fmt.Println("cache miss:", key)
	p, err := s.repo.GetProductByID(ctx, id)
	if err != nil {
		return nil, err
	}

	s.cache.Set(ctx, key, p, 5*time.Minute)
	return p, nil
}

func (s *ProductService) InvalidateCache(ctx context.Context, id int) error {
	key := fmt.Sprintf("Product %d", id)
	return s.cache.Delete(ctx, key)
}
