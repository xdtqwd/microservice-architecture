package service

import (
	"context"
	"fmt"
	"order-service/internal/domain"

)

type mockRepo struct {
	orders   []domain.Order
	products []domain.Product
	nextID   int
}
func newMockRepo() *mockRepo {
	return &mockRepo{
		nextID: 1,
		products: []domain.Product{ // ✅
			{ID: 1, Name: "MacBook Pro", Price: 150000, Stock: 10},
			{ID: 2, Name: "iPhone 15", Price: 80000, Stock: 0},
		},
	}
}

func (m *mockRepo) CreateOrder(ctx context.Context, items []domain.OrderItem) (int, error) {
	id := m.nextID
	m.nextID++
	m.orders = append(m.orders, domain.Order{ID: id, Status: "pending", Items: items})
	return id, nil
}

func (m *mockRepo) GetOrderByID(ctx context.Context, id int) (*domain.Order, error) {
	for _, o := range m.orders {
		if o.ID == id {
			return &o, nil
		}
	}
	return nil, fmt.Errorf("GetOrderByID: %w", domain.ErrOrderNotFound)
}

func (m *mockRepo) GetOrders(ctx context.Context, limit, offset int) ([]domain.Order, error) {
	if offset >= len(m.orders) {
		return []domain.Order{}, nil
	}
	end := offset + limit
	if end > len(m.orders) {
		end = len(m.orders)
	}
	return m.orders[offset:end], nil
}
func (m *mockRepo) CancelOrder(ctx context.Context, id int) (int, error) {
	for i, o := range m.orders {
		if o.ID == id {
			m.orders[i].Status = "cancelled"
			return id, nil
		}
	}
	return 0, nil
}
func (m *mockRepo) GetProducts(ctx context.Context) ([]domain.Product, error) {
	return m.products, nil
}

func (m *mockRepo) GetProductByID(ctx context.Context, id int) (*domain.Product, error) {
	for _, p := range m.products {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("GetOrderByID: %w", domain.ErrOrderNotFound)
}

func (m *mockRepo) InvalidateByID(ctx context.Context, id int) error {
	return nil
}
