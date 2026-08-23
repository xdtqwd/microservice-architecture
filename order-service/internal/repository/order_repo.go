package repository

import "github.com/jackc/pgx/v5/pgxpool"

type OrderRepo struct {
	pool *pgxpool.Pool
}

func NewOrderRepo(pool *pgxpool.Pool) *OrderRepo {
	return &OrderRepo{pool: pool}
}
