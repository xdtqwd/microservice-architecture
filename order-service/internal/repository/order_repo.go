package repository

import (
	"context"
	"order-service/internal/txm"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Invalidator interface {
	InvalidateByID(ctx context.Context, id int) error
}

type OrderRepo struct {
	pool        *pgxpool.Pool
	invalidator Invalidator
	logger      *zap.Logger
}

func NewOrderRepo(pool *pgxpool.Pool, invalidator Invalidator, logger *zap.Logger) *OrderRepo {
	return &OrderRepo{pool: pool, invalidator: invalidator, logger: logger}
}

// querier возвращает транзакцию из контекста или пул напрямую
func (r *OrderRepo) querier(ctx context.Context) Querier {
	if tx := txm.Extract(ctx); tx != nil {
		return tx
	}
	return r.pool
}
