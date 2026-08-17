package service

import (
	"context"
	"order-service/internal/repository"
)

type OrderService struct {
	repo *repository.Repository
}

func NewOrderService(repo *repository.Repository) *OrderService {
	return &OrderService{repo: repo}
}

func (s *OrderService) CreateOrder(ctx context.Context, items []repository.OrderItem) (int, error) {
	return s.repo.CreateOrder(ctx, items)
}

func (s *OrderService) GetOrders(ctx context.Context) ([]repository.Order, error) {
	return s.repo.GetOrders(ctx)
}

func (s *OrderService) GetOrderByID(ctx context.Context, id int) (*repository.Order, error) {
	return s.repo.GetOrderByID(ctx, id)
}

func (s *OrderService) CancelOrder(ctx context.Context, id int) (int, error) {
	return s.repo.CancelOrder(ctx, id)
}
