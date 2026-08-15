package repository

import (
	"context"
	"time"
)

type Order struct {
	ID        int
	Status    string
	CreatedAt time.Time
	Items     []OrderItem
}

type OrderItem struct {
	ID        int
	OrderID   int
	ProductID int
	Quantity  int
	Price     float64
}

func (r *Repository) CreateOrder(ctx context.Context, items []OrderItem) (int, error) {
	var orderID int
	err := r.pool.QueryRow(context.Background(),
		"INSERT INTO orders (status) VALUES ('pending') RETURNING id").
		Scan(&orderID)
	if err != nil {
		return 0, err
	}
	for _, item := range items {
		_, err = r.pool.Exec(context.Background(),
			`INSERT INTO order_items (order_id, product_id, quantity, price)
             VALUES ($1, $2, $3, $4)`,
			orderID, item.ProductID, item.Quantity, item.Price)
		if err != nil {
			return 0, err
		}
	}
	return orderID, nil
}

func (r *Repository) GetOrderByID(ctx context.Context, id int) (*Order, error) {
	var o Order
	err := r.pool.QueryRow(context.Background(),
		"SELECT id, status, created_at FROM orders WHERE id = $1", id).
		Scan(&o.ID, &o.Status, &o.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *Repository) GetOrders(ctx context.Context) ([]Order, error) {
	rows, err := r.pool.Query(context.Background(),
		"SELECT id, status, created_at FROM orders")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		err = rows.Scan(&o.ID, &o.Status, &o.CreatedAt)
		if err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, nil
}

func (r *Repository) CancelOrder(ctx context.Context, id int) (int, error) {
	var cancelledID int
	err := r.pool.QueryRow(ctx,
		"UPDATE orders SET status = $1 WHERE id = $2 RETURNING id",
		"cancelled", id).Scan(&cancelledID)
	if err != nil {
		return 0, err
	}
	return cancelledID, nil
}
