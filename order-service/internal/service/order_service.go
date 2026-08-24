package service

import (
	"context"
	"errors"
	"order-service/internal/repository"
)

type OrderService struct {
	repo OrderRepository
}

func NewOrderService(repo OrderRepository) *OrderService {
	return &OrderService{repo: repo}
}

func (s *OrderService) CreateOrder(ctx context.Context, items []repository.OrderItem) (int, error) {
	if len(items) == 0 {
		return 0, errors.New("order must have at least one item")
	}
	for _, item := range items {
		if item.Quantity <= 0 {
			return 0, errors.New("quantity must be greater than 0")
		}
	}
	return s.repo.CreateOrder(ctx, items)
}
func (s *OrderService) GetOrders(ctx context.Context) ([]repository.Order, error) {
	return s.repo.GetOrders(ctx)
}

func (s *OrderService) GetOrderByID(ctx context.Context, id int) (*repository.Order, error) {
	return s.repo.GetOrderByID(ctx, id)
}

func (s *OrderService) CancelOrder(ctx context.Context, id int) (int, error) {
	order, err := s.repo.GetOrderByID(ctx, id)
	if err != nil {
		return 0, err
	}
	if order == nil {
		return 0, errors.New("order not found")
	}
	if order.Status == "cancelled" {
		return 0, errors.New("order already cancelled")
	}
	return s.repo.CancelOrder(ctx, id)
}
