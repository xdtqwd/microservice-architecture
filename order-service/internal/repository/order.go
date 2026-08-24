package repository

import (
	"context"
	"order-service/internal/apperrors"
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

func (r *OrderRepo) CreateOrder(ctx context.Context, items []OrderItem) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var orderID int
	err = tx.QueryRow(ctx,
		"INSERT INTO orders (status) VALUES ('pending') RETURNING id").
		Scan(&orderID)
	if err != nil {
		return 0, err
	}

	for _, item := range items {
		tag, err := tx.Exec(ctx,
			"UPDATE products SET stock = stock - $1 WHERE id = $2 AND stock >= $1",
			item.Quantity, item.ProductID)
		if err != nil {
			return 0, err
		}
		if tag.RowsAffected() == 0 {
			return 0, apperrors.ErrInsufficientStock
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, quantity, price)
             VALUES ($1, $2, $3, $4)`,
			orderID, item.ProductID, item.Quantity, item.Price)
		if err != nil {
			return 0, err
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return 0, err
	}
	return orderID, nil
}

func (r *OrderRepo) GetOrderByID(ctx context.Context, id int) (*Order, error) {
	var o Order
	err := r.pool.QueryRow(ctx,
		"SELECT id, status, created_at FROM orders WHERE id = $1", id).
		Scan(&o.ID, &o.Status, &o.CreatedAt)
	if err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx,
		"SELECT id, order_id, product_id, quantity, price FROM order_items WHERE order_id = $1", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item OrderItem
		err = rows.Scan(&item.ID, &item.OrderID, &item.ProductID, &item.Quantity, &item.Price)
		if err != nil {
			return nil, err
		}
		o.Items = append(o.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *OrderRepo) GetOrders(ctx context.Context, limit, offset int) ([]Order, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT id, status, created_at FROM orders ORDER BY id LIMIT $1 OFFSET $2",
		limit, offset)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return orders, nil
}

func (r *OrderRepo) CancelOrder(ctx context.Context, id int) (int, error) {
	var cancelledID int
	err := r.pool.QueryRow(ctx,
		"UPDATE orders SET status = $1 WHERE id = $2 RETURNING id",
		"cancelled", id).Scan(&cancelledID)
	if err != nil {
		return 0, err
	}
	return cancelledID, nil
}
