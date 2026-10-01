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

func newPaymentRepo() *repository.PaymentRepo {
	return repository.NewPaymentRepo(testPool, zap.NewNop())
}

func orderStatus(t *testing.T, id int) string {
	t.Helper()
	var s string
	require.NoError(t, testPool.QueryRow(context.Background(), "SELECT status FROM orders WHERE id = $1", id).Scan(&s))
	return s
}

func paymentStatus(t *testing.T, id int64) string {
	t.Helper()
	var s string
	require.NoError(t, testPool.QueryRow(context.Background(), "SELECT status FROM payments WHERE id = $1", id).Scan(&s))
	return s
}

func newPendingOrder(t *testing.T) int {
	t.Helper()
	pid := seedProduct(t, "A", 1500, 10)
	return createOrder(t, newOrderRepo(), domain.OrderItem{ProductID: pid, Quantity: 2})
}

func TestPayment_Start_AmountFromItems(t *testing.T) {
	resetDB(t)
	orderID := newPendingOrder(t)

	id, amount, err := newPaymentRepo().Start(context.Background(), orderID)
	require.NoError(t, err)
	assert.Equal(t, "3000", amount.String())
	assert.Equal(t, "pending", paymentStatus(t, id))
}

func TestPayment_CannotPayCancelledOrder(t *testing.T) {
	resetDB(t)
	orderID := newPendingOrder(t)
	setStatus(t, orderID, "cancelled")

	_, _, err := newPaymentRepo().Start(context.Background(), orderID)
	assert.ErrorIs(t, err, domain.ErrInvalidStatusTransition)
	assert.Equal(t, 0, countRows(t, "payments"))
}

func TestPayment_SecondActivePayment_RejectedBySchema(t *testing.T) {
	resetDB(t)
	orderID := newPendingOrder(t)
	repo := newPaymentRepo()

	_, _, err := repo.Start(context.Background(), orderID)
	require.NoError(t, err)
	_, _, err = repo.Start(context.Background(), orderID)

	assert.ErrorIs(t, err, domain.ErrPaymentAlreadyActive)
	assert.Equal(t, 1, countRows(t, "payments"))
}

// Двойной клик из PAY-01: ровно один платёж, второй отбит индексом.
func TestPayment_ConcurrentStart_ExactlyOne(t *testing.T) {
	for round := 0; round < 20; round++ {
		resetDB(t)
		orderID := newPendingOrder(t)
		repo := newPaymentRepo()

		errs := runTogether(2, func(int) error {
			_, _, err := repo.Start(context.Background(), orderID)
			return err
		})

		ok, rejected := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case assert.ErrorIs(t, err, domain.ErrPaymentAlreadyActive):
				rejected++
			}
		}
		require.Equal(t, 1, ok, "раунд %d", round)
		require.Equal(t, 1, rejected, "раунд %d", round)
	}
}

func TestPayment_Succeeded_OrderPaidInSameTransaction(t *testing.T) {
	resetDB(t)
	orderID := newPendingOrder(t)
	repo := newPaymentRepo()
	id, _, err := repo.Start(context.Background(), orderID)
	require.NoError(t, err)

	require.Equal(t, "pending", orderStatus(t, orderID), "до подтверждения заказ не paid")
	require.NoError(t, repo.Transition(context.Background(), id, domain.PaymentSucceeded, "ch_1", "ok"))

	assert.Equal(t, "succeeded", paymentStatus(t, id))
	assert.Equal(t, "paid", orderStatus(t, orderID))
	assert.Equal(t, 2, countRows(t, "payment_events"), "start + succeeded")
}

func TestPayment_Failed_OrderStaysPending_NewPaymentAllowed(t *testing.T) {
	resetDB(t)
	orderID := newPendingOrder(t)
	repo := newPaymentRepo()
	id, _, err := repo.Start(context.Background(), orderID)
	require.NoError(t, err)

	require.NoError(t, repo.Transition(context.Background(), id, domain.PaymentFailed, "", "declined"))
	assert.Equal(t, "pending", orderStatus(t, orderID))

	_, _, err = repo.Start(context.Background(), orderID)
	assert.NoError(t, err, "после отказа можно попробовать снова")
}

func TestPayment_FromTerminal_ExplicitError(t *testing.T) {
	for _, terminal := range []domain.PaymentStatus{domain.PaymentFailed, domain.PaymentExpired} {
		t.Run(string(terminal), func(t *testing.T) {
			resetDB(t)
			orderID := newPendingOrder(t)
			repo := newPaymentRepo()
			id, _, err := repo.Start(context.Background(), orderID)
			require.NoError(t, err)
			require.NoError(t, repo.Transition(context.Background(), id, terminal, "", "test"))

			err = repo.Transition(context.Background(), id, domain.PaymentSucceeded, "ch_x", "late")

			assert.ErrorIs(t, err, domain.ErrPaymentTransition)
			assert.Equal(t, string(terminal), paymentStatus(t, id), "состояние не изменилось")
			assert.Equal(t, "pending", orderStatus(t, orderID), "заказ не стал paid")
		})
	}
}

func TestPayment_RefundedIsTerminal(t *testing.T) {
	resetDB(t)
	orderID := newPendingOrder(t)
	repo := newPaymentRepo()
	id, _, err := repo.Start(context.Background(), orderID)
	require.NoError(t, err)
	require.NoError(t, repo.Transition(context.Background(), id, domain.PaymentSucceeded, "ch_1", "ok"))
	require.NoError(t, repo.Transition(context.Background(), id, domain.PaymentRefunded, "", "refund"))

	err = repo.Transition(context.Background(), id, domain.PaymentSucceeded, "", "again")
	assert.ErrorIs(t, err, domain.ErrPaymentTransition)
}
