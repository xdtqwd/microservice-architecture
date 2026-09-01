package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Invalidator interface {
	InvalidateByID(ctx context.Context, id int) error
}

type OrderRepo struct {
	pool        *pgxpool.Pool
	db          Querier
	invalidator Invalidator
}

func NewOrderRepo(pool *pgxpool.Pool, invalidator Invalidator) *OrderRepo {
	return &OrderRepo{pool: pool, db: pool, invalidator: invalidator}
}
