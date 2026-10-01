package repository_test

import (
	"context"
	"testing"
	"time"

	"order-service/internal/domain"
	"order-service/internal/payment"
	"order-service/internal/reconciliation"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeLister struct{ charges []payment.ProviderCharge }

func (f fakeLister) ListCharges(context.Context, time.Time, time.Time) ([]payment.ProviderCharge, error) {
	return f.charges, nil
}

func pendingWithKey(t *testing.T, key string) (int, int64) {
	t.Helper()
	orderID := newPendingOrder(t) // 2 × 1500 = 3000
	p, err := newPaymentRepo().StartOrResume(context.Background(), orderID, key)
	require.NoError(t, err)
	return orderID, p.ID
}

func paidWithKey(t *testing.T, key string) (int, int64) {
	t.Helper()
	orderID, id := pendingWithKey(t, key)
	require.NoError(t, newPaymentRepo().Transition(context.Background(), id, domain.PaymentSucceeded, "ch_"+key, "ok"))
	return orderID, id
}

func charge(key, status string, amount int64) payment.ProviderCharge {
	return payment.ProviderCharge{Key: key, ChargeID: "ch_" + key, Amount: decimal.NewFromInt(amount), Status: status, CreatedAt: time.Now()}
}

func TestReconciliation_EveryKind(t *testing.T) {
	resetDB(t)
	ctx := context.Background()

	paidWithKey(t, "matched")
	_, missingProvID := paidWithKey(t, "missing-at-provider")
	_, amountID := paidWithKey(t, "amount")
	stuckOrder, stuckID := pendingWithKey(t, "pending-charged")
	_, declinedID := pendingWithKey(t, "pending-declined")
	refundOrder, refundPaymentID := paidWithKey(t, "refund")
	_, err := newOrderRepo().CancelOrder(ctx, refundOrder) // возврат ждёт провайдера
	require.NoError(t, err)
	_, failedID := pendingWithKey(t, "failed-charged")
	require.NoError(t, newPaymentRepo().Transition(ctx, failedID, domain.PaymentFailed, "", "declined"))

	provider := fakeLister{charges: []payment.ProviderCharge{
		charge("matched", "charged", 3000),
		// missing-at-provider — у провайдера записи нет
		charge("orphan", "charged", 777), // у нас такого платежа нет
		charge("amount", "charged", 2999),
		charge("pending-charged", "charged", 3000),
		charge("pending-declined", "declined", 3000),
		charge("refund", "refunded", 3000),
		charge("failed-charged", "charged", 3000),
	}}
	job := reconciliation.NewJob(testPool, provider, newPaymentRepo(), zap.NewNop())
	from, to := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)

	rep, err := job.Run(ctx, from, to)
	require.NoError(t, err)

	assert.Equal(t, 8, rep.Checked)
	assert.Equal(t, 1, rep.Matched)
	assert.Equal(t, 3, rep.AutoFixed, "безопасные починены")
	assert.Equal(t, 4, rep.Manual, "опасные оставлены человеку")
	assert.Equal(t, map[reconciliation.Kind]int{
		reconciliation.MissingAtProvider: 1,
		reconciliation.MissingAtOurs:     1,
		reconciliation.AmountMismatch:    1,
		reconciliation.StatusMismatch:    1,
	}, rep.ByKind)

	// безопасные — починены и приведены в согласованное состояние
	assert.Equal(t, "succeeded", paymentStatus(t, stuckID))
	assert.Equal(t, "paid", orderStatus(t, stuckOrder))
	assert.Equal(t, "failed", paymentStatus(t, declinedID))
	assert.Equal(t, "refunded", paymentStatus(t, refundPaymentID))
	var refundStatus string
	require.NoError(t, testPool.QueryRow(ctx, "SELECT status FROM refunds WHERE payment_id = $1", refundPaymentID).Scan(&refundStatus))
	assert.Equal(t, "succeeded", refundStatus)

	// опасные — не тронуты
	assert.Equal(t, "succeeded", paymentStatus(t, missingProvID))
	assert.Equal(t, "succeeded", paymentStatus(t, amountID))
	assert.Equal(t, "failed", paymentStatus(t, failedID))

	// в отчёте достаточно данных для разбора
	rows, err := testPool.Query(ctx, `
		SELECT kind, idempotency_key, COALESCE(our_amount::text, ''), COALESCE(provider_amount::text, ''), note
		FROM reconciliation_items WHERE run_id = $1 AND action = 'manual'`, rep.RunID)
	require.NoError(t, err)
	manual := map[string]string{}
	for rows.Next() {
		var kind, key, ourAmt, thAmt, note string
		require.NoError(t, rows.Scan(&kind, &key, &ourAmt, &thAmt, &note))
		assert.NotEmpty(t, note, key)
		manual[key] = kind
		if key == "amount" {
			assert.Equal(t, "3000.00", ourAmt)
			assert.Equal(t, "2999.00", thAmt)
		}
	}
	rows.Close()
	assert.Equal(t, map[string]string{
		"missing-at-provider": "missing_at_provider",
		"orphan":              "missing_at_ours",
		"amount":              "amount_mismatch",
		"failed-charged":      "status_mismatch",
	}, manual)

	assert.EqualValues(t, 1, testutil.ToFloat64(reconciliation.Discrepancies.WithLabelValues("amount_mismatch")))

	// повторный прогон: починенное совпадает, ручное остаётся, ничего не чиним дважды
	rep2, err := job.Run(ctx, from, to)
	require.NoError(t, err)
	assert.Equal(t, 0, rep2.AutoFixed)
	assert.Equal(t, 4, rep2.Manual)
	assert.Equal(t, 4, rep2.Matched)
}
