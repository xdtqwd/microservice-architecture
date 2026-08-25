package service

import (
	"context"
	"errors"
	"order-service/internal/domain"
)

const (
	defaultLimit = 50
	maxLimit     = 100
)

type OrderService struct {
	repo OrderRepository
}

func NewOrderService(repo OrderRepository) *OrderService {
	return &OrderService{repo: repo}
}

func (s *OrderService) CreateOrder(ctx context.Context, items []domain.CreateOrderItem) (int, error) {
	if len(items) == 0 {
		return 0, errors.New("order must have at least one item")
	}

	seen := make(map[int]bool)
	var orderItems []domain.OrderItem

	for _, item := range items {
		if item.Quantity <= 0 {
			return 0, errors.New("quantity must be greater than 0")
		}
		if seen[item.ProductID] {
			return 0, errors.New("duplicate product_id")
		}
		seen[item.ProductID] = true

		orderItems = append(orderItems, domain.OrderItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		})
	}

	return s.repo.CreateOrder(ctx, orderItems)
}

func (s *OrderService) GetOrders(ctx context.Context, limit, offset int) ([]domain.Order, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.GetOrders(ctx, limit, offset)
}

func (s *OrderService) GetOrderByID(ctx context.Context, id int) (*domain.Order, error) {
	return s.repo.GetOrderByID(ctx, id)
}

func (s *OrderService) CancelOrder(ctx context.Context, id int) (int, error) {
	order, err := s.repo.GetOrderByID(ctx, id)
	if err != nil {
		return 0, err
	}
	if order.Status == "cancelled" {
		return 0, domain.ErrOrderAlreadyCancelled
	}
	return s.repo.CancelOrder(ctx, id)
}
