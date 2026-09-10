package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"order-service/internal/domain"
)

func TestCancelOrder_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		expectErr   error
	}{
		{"delivered нельзя отменить", "delivered", domain.ErrInvalidStatusTransition},
		{"shipped нельзя отменить", "shipped", domain.ErrInvalidStatusTransition},
		{"cancelled нельзя отменить повторно", "cancelled", domain.ErrOrderAlreadyCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.orders = []domain.Order{
				{ID: 1, Status: tt.status},
			}
			svc := NewOrderService(repo)
			_, err := svc.CancelOrder(context.Background(), 1)
			assert.ErrorIs(t, err, tt.expectErr)
		})
	}
}

func TestCancelOrder_ValidTransitions(t *testing.T) {
	tests := []struct {
		name   string
		status string
	}{
		{"pending можно отменить", "pending"},
		{"paid можно отменить", "paid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.orders = []domain.Order{
				{ID: 1, Status: tt.status},
			}
			svc := NewOrderService(repo)
			_, err := svc.CancelOrder(context.Background(), 1)
			assert.NoError(t, err)
		})
	}
}

func TestCanTransition(t *testing.T) {
	assert.True(t, domain.CanTransition("pending", "paid"))
	assert.True(t, domain.CanTransition("pending", "cancelled"))
	assert.True(t, domain.CanTransition("paid", "shipped"))
	assert.True(t, domain.CanTransition("paid", "cancelled"))
	assert.True(t, domain.CanTransition("shipped", "delivered"))
	assert.False(t, domain.CanTransition("delivered", "cancelled"))
	assert.False(t, domain.CanTransition("shipped", "cancelled"))
	assert.False(t, domain.CanTransition("cancelled", "cancelled"))
}

func TestCancelOrder_ConcurrentDouble(t *testing.T) {
	repo := newMockRepo()
	repo.orders = []domain.Order{{ID: 1, Status: "pending"}}
	svc := NewOrderService(repo)
	ctx := context.Background()

	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := svc.CancelOrder(ctx, 1)
			results <- err
		}()
	}

	err1 := <-results
	err2 := <-results

	// ровно одна отмена успешна, вторая — ошибка
	errs := []error{err1, err2}
	successCount := 0
	for _, e := range errs {
		if e == nil {
			successCount++
		}
	}
	assert.Equal(t, 1, successCount, "ровно одна отмена должна пройти")

	// заказ отменён ровно один раз
	orders, _, _ := svc.GetOrders(ctx, 10, nil)
	for _, o := range orders {
		if o.ID == 1 {
			assert.Equal(t, "cancelled", o.Status)
		}
	}
}
