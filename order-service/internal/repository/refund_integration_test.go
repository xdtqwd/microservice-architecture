package repository_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"order-service/internal/domain"
	"order-service/internal/payment"
	"order-service/internal/worker"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeRefunder struct {
	calls     atomic.Int64
	failFirst int
	always    bool
	delay     time.Duration
}

func (f *fakeRefunder) Refund(_ context.Context, key, _ string, _ decimal.Decimal) (string, error) {
	n := int(f.calls.Add(1))
	time.Sleep(f.delay)
	if f.always || n <= f.failFirst {
		return "", payment.ErrUnavailable
	}
	return "rf_" + key, nil
}

// заказ на 2 шт. по 1500, оплачен
func newPaidOrder(t *testing.T) (orderID, productID int, paymentID int64) {
	t.Helper()
	productID = seedProduct(t, "A", 1500, 10)
	orderID = createOrder(t, newOrderRepo(), domain.OrderItem{ProductID: productID, Quantity: 2})
	repo := newPaymentRepo()
	paymentID, _, err := repo.Start(context.Background(), orderID)
	require.NoError(t, err)
	require.NoError(t, repo.Transition(context.Background(), paymentID, domain.PaymentSucceeded, "ch_1", "ok"))
	return orderID, productID, paymentID
}

func refundState(t *testing.T) (status string, attempts int) {
	t.Helper()
	require.NoError(t, testPool.QueryRow(context.Background(),
		"SELECT status, attempts FROM refunds ORDER BY id LIMIT 1").Scan(&status, &attempts))
	return status, attempts
}

func runWorkerUntil(t *testing.T, w *worker.RefundWorker, done func() bool) {
	t.Helper()
	for i := 0; i < 50 && !done(); i++ {
		require.NoError(t, w.ProcessDue(context.Background()))
		time.Sleep(10 * time.Millisecond)
	}
}

// ---------- отмена ----------

func TestCancel_PaidOrder_StockBack_RefundPending(t *testing.T) {
	resetDB(t)
	orderID, productID, paymentID := newPaidOrder(t)
	require.Equal(t, 8, stockOf(t, productID))

	_, err := newOrderRepo().CancelOrder(context.Background(), orderID)
	require.NoError(t, err)

	assert.Equal(t, 10, stockOf(t, productID), "остатки вернулись")
	assert.Equal(t, "cancelled", orderStatus(t, orderID))
	status, attempts := refundState(t)
	assert.Equal(t, "pending", status, "возврат записан и ждёт провайдера")
	assert.Zero(t, attempts, "к провайдеру из транзакции отмены не ходили")
	assert.Equal(t, "succeeded", paymentStatus(t, paymentID), "платёж станет refunded только после провайдера")
}

func TestCancel_UnpaidOrder_NoRefund(t *testing.T) {
	resetDB(t)
	orderID := newPendingOrder(t)

	_, err := newOrderRepo().CancelOrder(context.Background(), orderID)
	require.NoError(t, err)

	assert.Equal(t, "cancelled", orderStatus(t, orderID))
	assert.Equal(t, 0, countRows(t, "refunds"))
}

func TestCancel_PaymentInProgress_Rejected(t *testing.T) {
	resetDB(t)
	pid := seedProduct(t, "A", 1500, 10)
	orderID := createOrder(t, newOrderRepo(), domain.OrderItem{ProductID: pid, Quantity: 2})
	_, _, err := newPaymentRepo().Start(context.Background(), orderID)
	require.NoError(t, err)

	_, err = newOrderRepo().CancelOrder(context.Background(), orderID)

	assert.ErrorIs(t, err, domain.ErrPaymentInProgress)
	assert.Equal(t, "pending", orderStatus(t, orderID))
	assert.Equal(t, 8, stockOf(t, pid), "остатки не трогали")
}

func TestCancel_PaidOrderTwice_OneRefund(t *testing.T) {
	resetDB(t)
	orderID, _, _ := newPaidOrder(t)
	repo := newOrderRepo()

	_, err := repo.CancelOrder(context.Background(), orderID)
	require.NoError(t, err)
	_, err = repo.CancelOrder(context.Background(), orderID)
	assert.ErrorIs(t, err, domain.ErrOrderAlreadyCancelled)
	assert.Equal(t, 1, countRows(t, "refunds"))
}

// ---------- воркер ----------

func TestRefundWorker_ProviderDown_ThenUp_RefundGoesThrough(t *testing.T) {
	resetDB(t)
	orderID, _, paymentID := newPaidOrder(t)
	_, err := newOrderRepo().CancelOrder(context.Background(), orderID)
	require.NoError(t, err)

	prov := &fakeRefunder{failFirst: 2} // два раза лежит, потом поднялся
	w := worker.NewRefundWorker(testPool, prov, zap.NewNop(), worker.WithRefundRetry(time.Millisecond, 10*time.Millisecond, 8))

	runWorkerUntil(t, w, func() bool { s, _ := refundState(t); return s == "succeeded" })

	status, attempts := refundState(t)
	assert.Equal(t, "succeeded", status)
	assert.Equal(t, 3, attempts)
	assert.EqualValues(t, 3, prov.calls.Load())
	assert.Equal(t, "refunded", paymentStatus(t, paymentID))
}

func TestRefundWorker_Exhausted_ManualReview(t *testing.T) {
	resetDB(t)
	orderID, _, paymentID := newPaidOrder(t)
	_, err := newOrderRepo().CancelOrder(context.Background(), orderID)
	require.NoError(t, err)

	prov := &fakeRefunder{always: true}
	w := worker.NewRefundWorker(testPool, prov, zap.NewNop(), worker.WithRefundRetry(time.Millisecond, 10*time.Millisecond, 3))

	runWorkerUntil(t, w, func() bool { s, _ := refundState(t); return s == "manual_review" })

	status, attempts := refundState(t)
	assert.Equal(t, "manual_review", status, "не потерян — ждёт человека")
	assert.Equal(t, 3, attempts)
	assert.EqualValues(t, 3, prov.calls.Load())
	assert.Equal(t, "succeeded", paymentStatus(t, paymentID), "деньги не вернулись — платёж не refunded")
}

func TestRefundWorker_TwoWorkers_OneProviderCall(t *testing.T) {
	resetDB(t)
	orderID, _, _ := newPaidOrder(t)
	_, err := newOrderRepo().CancelOrder(context.Background(), orderID)
	require.NoError(t, err)

	prov := &fakeRefunder{delay: 100 * time.Millisecond}
	w1 := worker.NewRefundWorker(testPool, prov, zap.NewNop())
	w2 := worker.NewRefundWorker(testPool, prov, zap.NewNop())

	var wg sync.WaitGroup
	for _, w := range []*worker.RefundWorker{w1, w2} {
		wg.Add(1)
		go func(w *worker.RefundWorker) {
			defer wg.Done()
			assert.NoError(t, w.ProcessDue(context.Background()))
		}(w)
	}
	wg.Wait()

	assert.EqualValues(t, 1, prov.calls.Load(), "два инстанса не вернули деньги дважды")
	status, _ := refundState(t)
	assert.Equal(t, "succeeded", status)
}
