package repository

import (
	"context"
	"errors"
	"fmt"
	"order-service/internal/domain"
	"github.com/shopspring/decimal"
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

func (r *OrderRepo) CreateOrder(ctx context.Context, items []domain.OrderItem, idempotencyKey string) (int, bool, error) {
	fmt.Println("BEGIN ctx done:", ctx.Err())
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if idempotencyKey != "" {
		// вставляем заглушку — если ключ уже есть, читаем order_id
		_, err := r.pool.Exec(context.Background(),
			"INSERT INTO idempotency_keys (key, order_id) VALUES ($1, 0) ON CONFLICT (key) DO NOTHING",
			idempotencyKey)
		if err != nil {
			return 0, false, err
		}
		var existingID int
		err = r.pool.QueryRow(context.Background(),
			"SELECT order_id FROM idempotency_keys WHERE key = $1",
			idempotencyKey).Scan(&existingID)
		if err == nil && existingID > 0 {
			return existingID, true, nil
		}
	}

	var orderID int
	err = tx.QueryRow(ctx,
		"INSERT INTO orders (status) VALUES ('pending') RETURNING id").
		Scan(&orderID)
	if err != nil {
		return 0, false, err
	}

	for _, item := range items {
		var price decimal.Decimal
		err = tx.QueryRow(ctx,
			"SELECT price FROM products WHERE id = $1", item.ProductID).Scan(&price)
		if err != nil {
				return 0, false, fmt.Errorf("CreateOrder get price: %w", domain.ErrProductNotFound)
		}

		tag, err := tx.Exec(ctx,
			"UPDATE products SET stock = stock - $1 WHERE id = $2 AND stock >= $1",
			item.Quantity, item.ProductID)
		if err != nil {
			return 0, false, err
		}
		if tag.RowsAffected() == 0 {
			return 0, false, fmt.Errorf("CreateOrder: %w", domain.ErrInsufficientStock)
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, quantity, price)
             VALUES ($1, $2, $3, $4)`,
			orderID, item.ProductID, item.Quantity, price)
		if err != nil {
			return 0, false, err
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return 0, false, err
	}
	if idempotencyKey != "" {
		_, err = r.pool.Exec(context.Background(),
			"UPDATE idempotency_keys SET order_id = $1 WHERE key = $2",
			orderID, idempotencyKey)
		if err != nil {
			return 0, false, err
		}
	}
	return orderID, false, nil
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
	if len(orders) == 0 {
		return orders, nil, nil
	}

	// загружаем items одним запросом ANY($1)
	ids := make([]int, len(orders))
	for i, o := range orders {
		ids[i] = o.ID
	}
	itemRows, err := r.pool.Query(ctx,
		"SELECT id, order_id, product_id, quantity, price FROM order_items WHERE order_id = ANY($1) ORDER BY id",
		ids)
	if err != nil {
		return nil, nil, err
	}
	defer itemRows.Close()

	itemsByOrder := make(map[int][]domain.OrderItem)
	for itemRows.Next() {
		var item domain.OrderItem
		if err := itemRows.Scan(&item.ID, &item.OrderID, &item.ProductID, &item.Quantity, &item.Price); err != nil {
			return nil, nil, err
		}
		itemsByOrder[item.OrderID] = append(itemsByOrder[item.OrderID], item)
	}
	if err := itemRows.Err(); err != nil {
		return nil, nil, err
	}

	for i := range orders {
		orders[i].Items = itemsByOrder[orders[i].ID]
	}

	var nextCursor *domain.OrderCursor
	if len(orders) == limit {
		nextCursor = &domain.OrderCursor{AfterID: orders[len(orders)-1].ID}
	}
	return orders, nextCursor, nil
}

func (r *OrderRepo) CancelOrder(ctx context.Context, id int) (int, error) {
	var currentStatus string
	err := r.pool.QueryRow(ctx,
		"SELECT status FROM orders WHERE id = $1", id).Scan(&currentStatus)
	if err != nil {
		return 0, fmt.Errorf("CancelOrder: %w", domain.ErrOrderNotFound)
	}
	if !domain.CanTransition(currentStatus, "cancelled") {
		if currentStatus == "cancelled" {
			return 0, domain.ErrOrderAlreadyCancelled
		}
		return 0, fmt.Errorf("CancelOrder: %w", domain.ErrInvalidStatusTransition)
	}
	var cancelledID int
	err = r.pool.QueryRow(ctx,
		"UPDATE orders SET status = $1 WHERE id = $2 RETURNING id",
		"cancelled", id).Scan(&cancelledID)
	if err != nil {
		return 0, err
	}
	return cancelledID, nil
}
