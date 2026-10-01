package repository_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"order-service/internal/domain"
	"order-service/internal/payment"
	"order-service/internal/worker"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeChecker struct {
	calls    atomic.Int64
	status   payment.ChargeStatus
	chargeID string
	err      error
	delay    time.Duration
}

func (f *fakeChecker) ChargeStatus(context.Context, string) (payment.ChargeStatus, string, error) {
	f.calls.Add(1)
	time.Sleep(f.delay)
	return f.status, f.chargeID, f.err
}

// платёж в pending с ключом, «созданный» age назад
func newStuckPayment(t *testing.T, age time.Duration) (orderID int, paymentID int64) {
	t.Helper()
	pid := seedProduct(t, "A", 1500, 10)
	orderID = createOrder(t, newOrderRepo(), domain.OrderItem{ProductID: pid, Quantity: 1})
	p, err := newPaymentRepo().StartOrResume(context.Background(), orderID, "stuck-"+time.Now().Format("150405.000000"))
	require.NoError(t, err)
	_, err = testPool.Exec(context.Background(),
		"UPDATE payments SET created_at = NOW() - make_interval(secs => $2) WHERE id = $1", p.ID, age.Seconds())
	require.NoError(t, err)
	return orderID, p.ID
}

func newReconciler(c worker.ChargeChecker) *worker.ReconcileWorker {
	return worker.NewReconcileWorker(testPool, c, newPaymentRepo(), zap.NewNop(),
		worker.WithReconcileTiming(time.Minute, 10*time.Minute, time.Millisecond, 10*time.Millisecond, 3))
}

func reconcileState(t *testing.T, id int64) (attempts int, reason string) {
	t.Helper()
	var r *string
	require.NoError(t, testPool.QueryRow(context.Background(),
		"SELECT reconcile_attempts, manual_review_reason FROM payments WHERE id = $1", id).Scan(&attempts, &r))
	if r != nil {
		reason = *r
	}
	return attempts, reason
}

func TestReconcile_ChargedAtProvider_PaymentSucceeded_OrderPaid(t *testing.T) {
	resetDB(t)
	orderID, paymentID := newStuckPayment(t, 5*time.Minute)

	require.NoError(t, newReconciler(&fakeChecker{status: payment.ChargeSucceeded, chargeID: "ch_42"}).
		ProcessDue(context.Background()))

	assert.Equal(t, "succeeded", paymentStatus(t, paymentID))
	assert.Equal(t, "paid", orderStatus(t, orderID), "заказ приведён в согласованное состояние")
}

func TestReconcile_DeclinedAtProvider_PaymentFailed_OrderPending(t *testing.T) {
	resetDB(t)
	orderID, paymentID := newStuckPayment(t, 5*time.Minute)

	require.NoError(t, newReconciler(&fakeChecker{status: payment.ChargeDeclined}).ProcessDue(context.Background()))

	assert.Equal(t, "failed", paymentStatus(t, paymentID))
	assert.Equal(t, "pending", orderStatus(t, orderID), "можно оплатить снова")
}

func TestReconcile_NotFound_Young_KeepsWaiting(t *testing.T) {
	resetDB(t)
	_, paymentID := newStuckPayment(t, 5*time.Minute) // старше stuckAfter, младше expireAfter

	require.NoError(t, newReconciler(&fakeChecker{status: payment.ChargeNotFound}).ProcessDue(context.Background()))

	assert.Equal(t, "pending", paymentStatus(t, paymentID), "не догадываемся — ждём")
	attempts, reason := reconcileState(t, paymentID)
	assert.Equal(t, 1, attempts)
	assert.Empty(t, reason)
}

func TestReconcile_NotFound_Old_Expired(t *testing.T) {
	resetDB(t)
	orderID, paymentID := newStuckPayment(t, 11*time.Minute)

	require.NoError(t, newReconciler(&fakeChecker{status: payment.ChargeNotFound}).ProcessDue(context.Background()))

	assert.Equal(t, "expired", paymentStatus(t, paymentID))
	assert.Equal(t, "pending", orderStatus(t, orderID))
}

func TestReconcile_ProviderDown_ManualReviewAfterLimit(t *testing.T) {
	resetDB(t)
	_, paymentID := newStuckPayment(t, 5*time.Minute)
	checker := &fakeChecker{err: payment.ErrUnavailable}
	w := newReconciler(checker)

	for i := 0; i < 20; i++ {
		require.NoError(t, w.ProcessDue(context.Background()))
		if _, reason := reconcileState(t, paymentID); reason != "" {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}

	attempts, reason := reconcileState(t, paymentID)
	assert.Equal(t, 3, attempts)
	assert.True(t, strings.Contains(reason, "gave up after 3 attempts"), reason)
	assert.Equal(t, "pending", paymentStatus(t, paymentID), "не выдумываем исход — отдаём человеку")
	assert.EqualValues(t, 3, checker.calls.Load())

	require.NoError(t, w.ProcessDue(context.Background()))
	assert.EqualValues(t, 3, checker.calls.Load(), "после пометки воркер платёж не трогает")
}

func TestReconcile_FreshPayment_Untouched(t *testing.T) {
	resetDB(t)
	_, paymentID := newStuckPayment(t, 10*time.Second) // моложе stuckAfter
	checker := &fakeChecker{status: payment.ChargeSucceeded, chargeID: "ch_1"}

	require.NoError(t, newReconciler(checker).ProcessDue(context.Background()))

	assert.Zero(t, checker.calls.Load(), "идущий запрос не трогаем")
	assert.Equal(t, "pending", paymentStatus(t, paymentID))
}

func TestReconcile_TwoReplicas_OneProviderCall(t *testing.T) {
	resetDB(t)
	orderID, _ := newStuckPayment(t, 5*time.Minute)
	checker := &fakeChecker{status: payment.ChargeSucceeded, chargeID: "ch_1", delay: 100 * time.Millisecond}
	w1, w2 := newReconciler(checker), newReconciler(checker)

	var wg sync.WaitGroup
	for _, w := range []*worker.ReconcileWorker{w1, w2} {
		wg.Add(1)
		go func(w *worker.ReconcileWorker) {
			defer wg.Done()
			assert.NoError(t, w.ProcessDue(context.Background()))
		}(w)
	}
	wg.Wait()

	assert.EqualValues(t, 1, checker.calls.Load(), "реплики не мешают друг другу")
	assert.Equal(t, "paid", orderStatus(t, orderID))
}
