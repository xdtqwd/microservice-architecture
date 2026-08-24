package service

import (
	"context"
	"encoding/json"
	"order-service/internal/apperrors"
	"order-service/internal/repository"
	"time"

	"github.com/redis/go-redis/v9"
)

type mockRepo struct {
	orders   []repository.Order
	products []repository.Product
	nextID   int
}
type mockCache struct {
	data map[string][]byte
}

func newMockCache() *mockCache {
	return &mockCache{data: make(map[string][]byte)}
}

func (c *mockCache) Get(ctx context.Context, key string, dest interface{}) error {
	val, ok := c.data[key]
	if !ok {
		return redis.Nil
	}
	return json.Unmarshal(val, dest)
}
func (c *mockCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.data[key] = data
	return nil
}

func (c *mockCache) Delete(ctx context.Context, key string) error {
	delete(c.data, key)
	return nil
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
	for _, item := range items {
		for i, p := range m.products {
			if p.ID == item.ProductID {
				if p.Stock < item.Quantity {
					return 0, apperrors.ErrInsufficientStock
				}
				m.products[i].Stock -= item.Quantity
			}
		}
	}
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
func (m *mockRepo) GetProducts(ctx context.Context) ([]repository.Product, error) {
	return m.products, nil
}

func (m *mockRepo) GetProductByID(ctx context.Context, id int) (*repository.Product, error) {
	for _, p := range m.products {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, nil
}
