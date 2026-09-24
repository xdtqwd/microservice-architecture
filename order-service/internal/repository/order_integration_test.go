package repository_test

import (
	"context"
	"testing"

	"order-service/internal/domain"
	"order-service/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type noopInvalidator struct{}

func (noopInvalidator) InvalidateByID(context.Context, int) error { return nil }

func newRepo() *repository.OrderRepo {
	return repository.NewOrderRepo(testPool, noopInvalidator{}, zap.NewNop())
}

func TestCreateAndCancel_RestoresStock(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	pid := seedProduct(t, "MacBook", 1000, 10)
	repo := newRepo()

	orderID, _, err := repo.CreateOrder(ctx,
		[]domain.OrderItem{{ProductID: pid, Quantity: 4}}, "")
	require.NoError(t, err)
	assert.Equal(t, 6, stockOf(t, pid), "создание заказа списывает остаток")

	_, err = repo.CancelOrder(ctx, orderID)
	require.NoError(t, err)
	assert.Equal(t, 10, stockOf(t, pid), "отмена возвращает остаток")

	_, err = repo.CancelOrder(ctx, orderID)
	assert.ErrorIs(t, err, domain.ErrOrderAlreadyCancelled)
	assert.Equal(t, 10, stockOf(t, pid), "повторная отмена не возвращает второй раз")
}

func TestCreateOrder_InsufficientStock(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	pid := seedProduct(t, "iPhone", 500, 1)
	repo := newRepo()

	_, _, err := repo.CreateOrder(ctx,
		[]domain.OrderItem{{ProductID: pid, Quantity: 2}}, "")
	assert.ErrorIs(t, err, domain.ErrInsufficientStock)
	assert.Equal(t, 1, stockOf(t, pid), "при нехватке остаток не меняется")
}
