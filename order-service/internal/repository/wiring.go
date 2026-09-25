package repository

import (
	"order-service/internal/cache"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func NewRepositories(pool *pgxpool.Pool, c *cache.RedisCache, logger *zap.Logger) (*OrderRepo, *L1ProductRepo) {
	productRepo := NewProductRepo(pool)
	l2 := NewCachedProductRepo(productRepo, c, logger)
	l1 := NewL1ProductRepo(l2)
	return NewOrderRepo(pool, l1, logger), l1
}
