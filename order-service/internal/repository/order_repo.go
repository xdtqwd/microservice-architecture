package repository

import "github.com/jackc/pgx/v5/pgxpool"

type OrderRepo struct {
	pool        *pgxpool.Pool
	db          Querier
	invalidator Invalidator
}

func NewOrderRepo(pool *pgxpool.Pool, invalidator Invalidator) *OrderRepo {
	return &OrderRepo{pool: pool, db: pool, invalidator: invalidator}
}
