package repository

import (
	"context"
	"errors"
	"fmt"
	"order-service/internal/domain"
	"time"

	"github.com/jackc/pgx/v5"
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

func (r *OrderRepo) CreateOrder(ctx context.Context, items []domain.OrderItem) (int, error) {
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
		var price float64
		err = tx.QueryRow(ctx,
			"SELECT price FROM products WHERE id = $1", item.ProductID).Scan(&price)
		if err != nil {
			return 0, fmt.Errorf("CreateOrder get price: %w", domain.ErrProductNotFound)
		}

		tag, err := tx.Exec(ctx,
			"UPDATE products SET stock = stock - $1 WHERE id = $2 AND stock >= $1",
			item.Quantity, item.ProductID)
		if err != nil {
			return 0, err
		}
		if tag.RowsAffected() == 0 {
			return 0, fmt.Errorf("CreateOrder: %w", domain.ErrInsufficientStock)
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, quantity, price)
             VALUES ($1, $2, $3, $4)`,
			orderID, item.ProductID, item.Quantity, price)
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

func (r *OrderRepo) GetOrderByID(ctx context.Context, id int) (*domain.Order, error) {
	var o domain.Order
	err := r.pool.QueryRow(ctx,
		"SELECT id, status, created_at FROM orders WHERE id = $1", id).
		Scan(&o.ID, &o.Status, &o.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("GetOrderByID: %w", domain.ErrOrderNotFound)
		}
		return nil, fmt.Errorf("GetOrderByID: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		"SELECT id, order_id, product_id, quantity, price FROM order_items WHERE order_id = $1", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item domain.OrderItem
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

func (r *OrderRepo) GetOrders(ctx context.Context, limit int, cursor *domain.OrderCursor) ([]domain.Order, *domain.OrderCursor, error) {
	var rows pgx.Rows
	var err error
	if cursor != nil && cursor.AfterID > 0 {
		rows, err = r.pool.Query(ctx,
			"SELECT id, status, created_at FROM orders WHERE id < $1 ORDER BY id DESC LIMIT $2",
			cursor.AfterID, limit)
	} else {
		rows, err = r.pool.Query(ctx,
			"SELECT id, status, created_at FROM orders ORDER BY id DESC LIMIT $1",
			limit)
	}
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var orders []domain.Order
	for rows.Next() {
		var o domain.Order
		err = rows.Scan(&o.ID, &o.Status, &o.CreatedAt)
		if err != nil {
			return nil, nil, err
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	var nextCursor *domain.OrderCursor
	if len(orders) == limit {
		nextCursor = &domain.OrderCursor{AfterID: orders[len(orders)-1].ID}
	}
	return orders, nextCursor, nil
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
