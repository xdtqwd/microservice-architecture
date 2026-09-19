package repository

import (
	"context"
	"order-service/internal/txm"

	"github.com/jackc/pgx/v5/pgxpool"
)

type OrderRepo struct {
	pool *pgxpool.Pool
}

func NewOrderRepo(pool *pgxpool.Pool) *OrderRepo {
	return &OrderRepo{pool: pool}
}

// querier возвращает транзакцию из контекста или пул напрямую
func (r *OrderRepo) querier(ctx context.Context) Querier {
	if tx := txm.Extract(ctx); tx != nil {
		return tx
	}
	return r.pool
}
