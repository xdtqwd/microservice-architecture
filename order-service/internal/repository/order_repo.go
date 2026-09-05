package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Invalidator interface {
	InvalidateByID(ctx context.Context, id int) error
}

type OrderRepo struct {
	pool        Beginner
	db          Querier
	invalidator Invalidator
}

func NewOrderRepo(pool Beginner, invalidator Invalidator) *OrderRepo {
	return &OrderRepo{pool: pool, db: pool.(Querier), invalidator: invalidator}
}
