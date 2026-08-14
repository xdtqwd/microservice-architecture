package repository

import (
	"context"
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

func CreateOrder(conn *pgx.Conn, items []OrderItem) (int, error) {
	var orderID int
	err := conn.QueryRow(context.Background(),
		"INSERT INTO orders (status) VALUES ('pending') RETURNING id").Scan(&orderID)
	if err != nil {
		return 0, err
	}
	for _, item := range items {
		_, err := conn.Exec(context.Background(),
			`INSERT INTO order_items (order_id, product_id, quantity, price)
             VALUES ($1, $2, $3, $4)`,
			orderID, item.ProductID, item.Quantity, item.Price)
		if err != nil {
			return 0, err
		}
	}
	return orderID, nil
}

func GetOrderByID(conn *pgx.Conn, id int) (*Order, error) {
	var o Order
	err := conn.QueryRow(context.Background(),
		"SELECT id, status, created_at FROM orders WHERE id = $1", id).Scan(&o.ID, &o.Status, &o.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func GetOrders(conn *pgx.Conn) ([]Order, error) {
	rows, err := conn.Query(context.Background(),
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

func CancelOrder(conn *pgx.Conn, id int) error {
	_, err := conn.Exec(context.Background(),
		"UPDATE orders SET status = $1 WHERE id = $2",
		"canceled", id)
	return err
}
