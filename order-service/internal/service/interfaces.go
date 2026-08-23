package service

import (
	"context"
	"order-service/internal/repository"
	"time"
)

type OrderRepository interface {
	CreateOrder(ctx context.Context, items []repository.OrderItem) (int, error)
	GetOrderByID(ctx context.Context, id int) (*repository.Order, error)
	GetOrders(ctx context.Context, limit, offset int) ([]repository.Order, error)
	CancelOrder(ctx context.Context, id int) (int, error)
	GetProductByID(ctx context.Context, id int) (*repository.Product, error)
}
type ProductRepository interface {
	GetProducts(ctx context.Context) ([]repository.Product, error)
	GetProductByID(ctx context.Context, id int) (*repository.Product, error)
}

type Cache interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}
