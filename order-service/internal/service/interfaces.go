package service

import (
	"context"
	"order-service/internal/repository"
)

type OrderRepository interface {
	CreateOrder(ctx context.Context, items []repository.OrderItem) (int, error)
	GetOrderByID(ctx context.Context, id int) (*repository.Order, error)
	GetOrders(ctx context.Context) ([]repository.Order, error)
	CancelOrder(ctx context.Context, id int) (int, error)
}
