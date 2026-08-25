package service

import (
	"context"
	"order-service/internal/domain"
	"time"
)

type OrderRepository interface {
	CreateOrder(ctx context.Context, items []domain.OrderItem) (int, error)
	GetOrderByID(ctx context.Context, id int) (*domain.Order, error)
	GetOrders(ctx context.Context, limit, offset int) ([]domain.Order, error)
	CancelOrder(ctx context.Context, id int) (int, error)
}

type ProductRepository interface {
	GetProducts(ctx context.Context) ([]domain.Product, error)
	GetProductByID(ctx context.Context, id int) (*domain.Product, error)
}

type Cache interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}
