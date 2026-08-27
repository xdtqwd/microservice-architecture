package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"order-service/internal/domain"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const productTTL = 5 * time.Minute

type CachedProductRepo struct {
	repo   *ProductRepo
	client *redis.Client
	logger *zap.Logger
}

func NewCachedProductRepo(repo *ProductRepo, client *redis.Client, logger *zap.Logger) *CachedProductRepo {
	return &CachedProductRepo{repo: repo, client: client, logger: logger}
}

func (r *CachedProductRepo) GetProducts(ctx context.Context) ([]domain.Product, error) {
	return r.repo.GetProducts(ctx)
}

func (r *CachedProductRepo) GetProductByID(ctx context.Context, id int) (*domain.Product, error) {
	key := fmt.Sprintf("product:%d", id)

	val, err := r.client.Get(ctx, key).Result()
	if err == nil {
		r.logger.Info("cache hit", zap.String("key", key))
		var p domain.Product
		if err := json.Unmarshal([]byte(val), &p); err != nil {
			return nil, err
		}
		return &p, nil
	}
	if !errors.Is(err, redis.Nil) {
		r.logger.Error("redis error", zap.Error(err))
	}
	r.logger.Info("cache miss", zap.String("key", key))

	p, err := r.repo.GetProductByID(ctx, id)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(p)
	if err == nil {
		if err := r.client.Set(ctx, key, data, productTTL).Err(); err != nil {
			r.logger.Error("cache set error", zap.Error(err))
		}
	}
	return p, nil
}

func (r *CachedProductRepo) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *CachedProductRepo) InvalidateByID(ctx context.Context, id int) error {
	return r.client.Del(ctx, fmt.Sprintf("product:%d", id)).Err()
}
