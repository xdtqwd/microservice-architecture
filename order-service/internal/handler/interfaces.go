package handler

import (
	"context"
	"order-service/internal/repository"
	"order-service/internal/service"
)

type OrderService interface {
	CreateOrder(ctx context.Context, items []service.CreateOrderItem) (int, error)
	GetOrders(ctx context.Context, limit, offset int) ([]repository.Order, error)
	GetOrderByID(ctx context.Context, id int) (*repository.Order, error)
	CancelOrder(ctx context.Context, id int) (int, error)
}

type ProductService interface {
	GetProducts(ctx context.Context) ([]repository.Product, error)
	GetProductByID(ctx context.Context, id int) (*repository.Product, error)
	InvalidateCache(ctx context.Context, id int) error
}
