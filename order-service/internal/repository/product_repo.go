package repository

import "context"

type ProductRepo struct {
	db Querier
}

func NewProductRepo(db Querier) *ProductRepo {
	return &ProductRepo{db: db}
}

func (r *ProductRepo) InvalidateByID(ctx context.Context, id int) error {
	return nil
}
