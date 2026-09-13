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
