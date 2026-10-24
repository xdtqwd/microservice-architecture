package service

import (
	"context"
	"testing"

	"order-service/internal/domain"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// Мутант :62 (<= 0 -> < 0): граница — ровно ноль, а не только отрицательные.
func TestCreateOrder_ZeroQuantity_Rejected(t *testing.T) {
	repo := newMockRepo()
	svc := NewOrderService(repo, nil, zap.NewNop(), nil, nil)

	_, _, err := svc.CreateOrder(context.Background(),
		[]domain.CreateOrderItem{{ProductID: 1, Quantity: 0}}, "")

	assert.Error(t, err)
	assert.Empty(t, repo.orders, "заказ на 0 штук не должен дойти до репозитория")
}

// Мутант :111 (<= 0 -> < 0): limit=0 должен превратиться в значение по умолчанию.
func TestGetOrders_ZeroOrNegativeLimit_UsesDefault(t *testing.T) {
	for _, limit := range []int{0, -5} {
		repo := newMockRepo()
		svc := NewOrderService(repo, nil, zap.NewNop(), nil, nil)

		_, _, err := svc.GetOrders(context.Background(), limit, nil)

		assert.NoError(t, err)
		assert.Equal(t, defaultLimit, repo.lastLimit, "limit=%d", limit)
	}
}

// Граница потолка: ровно maxLimit пропускается как есть, на единицу больше — срезается.
// Мутант :114 (> -> >=) эквивалентный: при limit == maxLimit обе ветки дают maxLimit.
func TestGetOrders_LimitCeiling(t *testing.T) {
	cases := map[int]int{maxLimit - 1: maxLimit - 1, maxLimit: maxLimit, maxLimit + 1: maxLimit, 100000: maxLimit}
	for in, want := range cases {
		repo := newMockRepo()
		svc := NewOrderService(repo, nil, zap.NewNop(), nil, nil)

		_, _, err := svc.GetOrders(context.Background(), in, nil)

		assert.NoError(t, err)
		assert.Equal(t, want, repo.lastLimit, "limit=%d", in)
	}
}
