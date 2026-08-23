package handler

import (
	"context"
	"order-service/internal/domain"
	"order-service/internal/service"
)

type OrderService interface {
	CreateOrder(ctx context.Context, items []service.CreateOrderItem) (int, error)
	GetOrders(ctx context.Context, limit, offset int) ([]domain.Order, error)
	GetOrderByID(ctx context.Context, id int) (*domain.Order, error)
	CancelOrder(ctx context.Context, id int) (int, error)
}

type ProductService interface {
	GetProducts(ctx context.Context) ([]domain.Product, error)
	GetProductByID(ctx context.Context, id int) (*domain.Product, error)
	InvalidateCache(ctx context.Context, id int) error
}
