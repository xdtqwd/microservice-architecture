package service

import (
	"context"
	"testing"

	"order-service/internal/domain"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// FIX-01: правила заказа — ошибка клиента (400), а не авария сервиса (500).
func TestCreateOrder_ValidationErrorsAreInvalidRequest(t *testing.T) {
	cases := map[string][]domain.CreateOrderItem{
		"пустой заказ": {},
		"quantity 0":   {{ProductID: 1, Quantity: 0}},
		"quantity < 0": {{ProductID: 1, Quantity: -1}},
		"дубль товара": {{ProductID: 1, Quantity: 1}, {ProductID: 1, Quantity: 2}},
	}
	for name, items := range cases {
		t.Run(name, func(t *testing.T) {
			repo := newMockRepo()
			svc := NewOrderService(repo, nil, zap.NewNop(), nil, nil)

			_, _, err := svc.CreateOrder(context.Background(), items, "")

			assert.ErrorIs(t, err, domain.ErrInvalidRequest)
			assert.Empty(t, repo.orders, "до репозитория не дошло")
		})
	}
}
