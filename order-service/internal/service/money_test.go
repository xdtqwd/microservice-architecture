package service

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"order-service/internal/domain"
)

// TestCreateOrder_PriceDecimalPrecision проверяет что цена в заказе
// не теряет точность — в отличие от float64.
func TestCreateOrder_PriceDecimalPrecision(t *testing.T) {
	repo := newMockRepo()
	repo.products = append(repo.products, domain.Product{
		ID:    3,
		Name:  "Dime",
		Price: decimal.NewFromFloat(0.1),
		Stock: 10,
	})

	svc := NewOrderService(repo, nil)
	ctx := context.Background()

	_, _, err := svc.CreateOrder(ctx, []domain.CreateOrderItem{
		{ProductID: 3, Quantity: 1},
	}, "")
	assert.NoError(t, err)

	orders, _, err := svc.GetOrders(ctx, 10, nil)
	assert.NoError(t, err)

	for _, o := range orders {
		for _, item := range o.Items {
			if item.ProductID == 3 {
				// float64 дал бы 0.10000000000000001
				// decimal даёт точно 0.10
				assert.Equal(t, "0.10", item.Price.StringFixed(2))
			}
		}
	}
}
