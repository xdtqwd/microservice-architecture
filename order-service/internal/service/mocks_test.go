package service

import (
	"context"
	"order-service/internal/repository"
)

type mockRepo struct {
	orders   []repository.Order
	products []repository.Product
	nextID   int
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		nextID: 1,
		products: []repository.Product{
			{ID: 1, Name: "MacBook Pro", Price: 150000, Stock: 10},
			{ID: 2, Name: "iPhone 15", Price: 80000, Stock: 0},
		},
	}
}

func (m *mockRepo) CreateOrder(ctx context.Context, items []repository.OrderItem) (int, error) {
	id := m.nextID
	m.nextID++
	m.orders = append(m.orders, repository.Order{ID: id, Status: "pending", Items: items})
	return id, nil
}

func (m *mockRepo) GetOrderByID(ctx context.Context, id int) (*repository.Order, error) {
	for _, o := range m.orders {
		if o.ID == id {
			return &o, nil
		}
	}
	return nil, nil
}

func (m *mockRepo) GetOrders(ctx context.Context) ([]repository.Order, error) {
	return m.orders, nil
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
