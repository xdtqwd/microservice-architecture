package service

import (
	"context"
	"errors"
	"order-service/internal/domain"
	"golang.org/x/sync/singleflight"
)

const (
	defaultLimit = 50
	maxLimit     = 100
)

type OrderService struct {
	repo  OrderRepository
	group singleflight.Group
}

func NewOrderService(repo OrderRepository) *OrderService {
	return &OrderService{repo: repo}
}

func (s *OrderService) CreateOrder(ctx context.Context, items []domain.CreateOrderItem, idempotencyKey string) (int, error) {
	if idempotencyKey != "" {
		val, err, _ := s.group.Do(idempotencyKey, func() (interface{}, error) {
			return s.createOrder(context.Background(), items, idempotencyKey)
		})
		if err != nil {
			return 0, err
		}
		return val.(int), nil
	}
	return s.createOrder(ctx, items, "")
}

func (s *OrderService) createOrder(ctx context.Context, items []domain.CreateOrderItem, idempotencyKey string) (int, error) {
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

	return s.repo.CreateOrder(ctx, orderItems, idempotencyKey)
}

func (s *OrderService) GetOrders(ctx context.Context, limit int, cursor *domain.OrderCursor) ([]domain.Order, *domain.OrderCursor, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return s.repo.GetOrders(ctx, limit, cursor)
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
