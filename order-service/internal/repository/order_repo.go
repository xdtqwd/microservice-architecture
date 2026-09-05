package repository

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Invalidator interface {
	InvalidateByID(ctx context.Context, id int) error
}

type OrderRepo struct {
	pool        *pgxpool.Pool
	invalidator Invalidator
}

func NewOrderRepo(pool *pgxpool.Pool, invalidator Invalidator) *OrderRepo {
	return &OrderRepo{pool: pool, invalidator: invalidator}
}
