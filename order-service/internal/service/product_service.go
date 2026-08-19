package service

import (
	"context"
	"fmt"
	"order-service/internal/cache"
	"order-service/internal/repository"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type ProductService struct {
	repo   *repository.Repository
	cache  *cache.RedisCache
	logger *zap.Logger
}

func NewProductService(repo *repository.Repository, cache *cache.RedisCache, logger *zap.Logger) *ProductService {
	return &ProductService{repo: repo, cache: cache, logger: logger}
}

func (s *ProductService) GetProducts(ctx context.Context) ([]repository.Product, error) {
	return s.repo.GetProducts(ctx)
}

func (s *ProductService) GetProductByID(ctx context.Context, id int) (*repository.Product, error) {
	key := fmt.Sprintf("product:%d", id)
	var product repository.Product

	err := s.cache.Get(ctx, key, &product)
	if err == nil {
		s.logger.Info("cache hit:", zap.String("key", key))
		return &product, nil
	}
	if err != redis.Nil {
		s.logger.Info("redis error:", zap.Error(err))
	}
	s.logger.Info("cache miss:", zap.String("key", key))
	p, err := s.repo.GetProductByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.cache.Set(ctx, key, p, 5*time.Minute); err != nil {
		s.logger.Error("cache set error", zap.Error(err))
	}
	return p, nil
}

func (s *ProductService) InvalidateCache(ctx context.Context, id int) error {
	key := fmt.Sprintf("product:%d", id)
	return s.cache.Delete(ctx, key)
}
