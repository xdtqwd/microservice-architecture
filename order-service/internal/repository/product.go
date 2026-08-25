package repository

import (
	"context"
	"errors"
	"fmt"
	"order-service/internal/domain"

	"github.com/jackc/pgx/v5"
)

type Product struct {
	ID    int
	Name  string
	Price float64
	Stock int
}

func (r *ProductRepo) GetProducts(ctx context.Context) ([]domain.Product, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT id, name, price, stock FROM products")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []domain.Product
	for rows.Next() {
		var p domain.Product
		err = rows.Scan(&p.ID, &p.Name, &p.Price, &p.Stock)
		if err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return products, nil
}

func (r *ProductRepo) GetProductByID(ctx context.Context, id int) (*domain.Product, error) {
	var p domain.Product
	err := r.pool.QueryRow(ctx,
		"SELECT id, name, price, stock FROM products WHERE id = $1", id).
		Scan(&p.ID, &p.Name, &p.Price, &p.Stock)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("GetProductByID: %w", domain.ErrProductNotFound)
		}
		return nil, fmt.Errorf("GetProductByID: %w", err)
	}
	return &p, nil
}
