package repository

import "context"

type Product struct {
	ID    int
	Name  string
	Price float64
	Stock int
}

func (r *Repository) GetProducts(ctx context.Context) ([]Product, error) {
	rows, err := r.pool.Query(context.Background(),
		"SELECT id, name, price, stock FROM products")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		err = rows.Scan(&p.ID, &p.Name, &p.Price, &p.Stock)
		if err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, nil
}

func (r *Repository) GetProductByID(ctx context.Context, id int) (*Product, error) {
	var p Product
	err := r.pool.QueryRow(context.Background(),
		"SELECT id, name, price, stock FROM products WHERE id = $1", id).
		Scan(&p.ID, &p.Name, &p.Price, &p.Stock)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
