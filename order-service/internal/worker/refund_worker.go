package worker

import (
	"context"
	"fmt"
	"time"

	"order-service/internal/domain"
	"order-service/internal/txm"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

var RefundsManualReview = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "refunds_manual_review_total",
	Help: "Refunds that exhausted retries and need manual handling",
})

func init() { prometheus.MustRegister(RefundsManualReview) }

type RefundProvider interface {
	Refund(ctx context.Context, key, chargeID string, amount decimal.Decimal) (string, error)
}

type RefundWorker struct {
	pool        *pgxpool.Pool
	provider    RefundProvider
	logger      *zap.Logger
	interval    time.Duration
	lease       time.Duration // на сколько возврат занят одной попыткой
	retryBase   time.Duration // первая пауза после неудачи, дальше удваивается
	retryCap    time.Duration
	maxAttempts int
	batch       int
}

type RefundOption func(*RefundWorker)

// WithRefundRetry — для тестов: короткие паузы и маленький лимит.
func WithRefundRetry(base, cap time.Duration, maxAttempts int) RefundOption {
	return func(w *RefundWorker) { w.retryBase, w.retryCap, w.maxAttempts = base, cap, maxAttempts }
}

func NewRefundWorker(pool *pgxpool.Pool, provider RefundProvider, logger *zap.Logger, opts ...RefundOption) *RefundWorker {
	w := &RefundWorker{
		pool: pool, provider: provider, logger: logger,
		interval:  time.Second,
		lease:     30 * time.Second,
		retryBase: 2 * time.Second,
		retryCap:  time.Minute,
		// 2+4+8+16+32+60+60+60 ≈ 4 минуты попыток, дальше — человеку
		maxAttempts: 8,
		batch:       10,
	}
	for _, o := range opts {
		o(w)
	}
	return w
}

func (w *RefundWorker) Run(ctx context.Context) {
	w.logger.Info("refund worker started")
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("refund worker stopped")
			return
		case <-t.C:
			if err := w.ProcessDue(ctx); err != nil {
				w.logger.Warn("refund worker", zap.Error(err))
			}
		}
	}
}

type dueRefund struct {
	id, paymentID int64
	amount        decimal.Decimal
	attempts      int
	chargeID      string
}

// ProcessDue обрабатывает возвраты, у которых подошло время попытки.
func (w *RefundWorker) ProcessDue(ctx context.Context) error {
	// 1. Арендуем пачку одним запросом: SKIP LOCKED — несколько инстансов
	//    не возьмут один возврат; next_attempt_at в будущее — пока идёт
	//    вызов провайдера, другой воркер этот возврат не тронет.
	rows, err := w.pool.Query(ctx, `
		UPDATE refunds r
		SET attempts = r.attempts + 1,
		    next_attempt_at = NOW() + make_interval(secs => $2),
		    updated_at = NOW()
		FROM payments p
		WHERE p.id = r.payment_id
		  AND r.id IN (
		      SELECT id FROM refunds
		      WHERE status = 'pending' AND next_attempt_at <= NOW()
		      ORDER BY id LIMIT $1
		      FOR UPDATE SKIP LOCKED)
		RETURNING r.id, r.payment_id, r.amount, r.attempts, COALESCE(p.provider_payment_id, '')`,
		w.batch, w.lease.Seconds())
	if err != nil {
		return err
	}
	var due []dueRefund
	for rows.Next() {
		var d dueRefund
		if err := rows.Scan(&d.id, &d.paymentID, &d.amount, &d.attempts, &d.chargeID); err != nil {
			rows.Close()
			return err
		}
		due = append(due, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// 2. Провайдер — вне транзакции. Ключ refund-<id> — один на возврат,
	//    повтор не вернёт деньги второй раз.
	for _, d := range due {
		providerID, perr := w.provider.Refund(ctx, fmt.Sprintf("refund-%d", d.id), d.chargeID, d.amount)

		// 3. Результат — короткой транзакцией на отвязанном контексте.
		rctx, cancel := txm.Detached()
		if perr == nil {
			err = w.markSucceeded(rctx, d, providerID)
		} else {
			err = w.markFailed(rctx, d, perr)
		}
		cancel()
		if err != nil {
			w.logger.Error("refund: failed to record result", zap.Int64("refund_id", d.id), zap.Error(err))
		}
	}
	return nil
}

func (w *RefundWorker) markSucceeded(ctx context.Context, d dueRefund, providerID string) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE refunds SET status = 'succeeded', provider_refund_id = $2, last_error = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'pending'`, d.id, providerID); err != nil {
		return err
	}

	var from string
	if err := tx.QueryRow(ctx, "SELECT status FROM payments WHERE id = $1 FOR UPDATE", d.paymentID).Scan(&from); err != nil {
		return err
	}
	if !domain.CanTransitionPayment(domain.PaymentStatus(from), domain.PaymentRefunded) {
		return fmt.Errorf("%w: %s -> refunded", domain.ErrPaymentTransition, from)
	}
	if _, err := tx.Exec(ctx,
		"UPDATE payments SET status = 'refunded', updated_at = NOW() WHERE id = $1", d.paymentID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO payment_events (payment_id, from_status, to_status, reason)
		VALUES ($1, $2, 'refunded', $3)`, d.paymentID, from, "refund "+providerID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	w.logger.Info("refund succeeded", zap.Int64("refund_id", d.id), zap.String("provider_refund_id", providerID),
		zap.Int("attempt", d.attempts))
	return nil
}

func (w *RefundWorker) markFailed(ctx context.Context, d dueRefund, perr error) error {
	if d.attempts >= w.maxAttempts {
		_, err := w.pool.Exec(ctx, `
			UPDATE refunds SET status = 'manual_review', last_error = $2, updated_at = NOW()
			WHERE id = $1 AND status = 'pending'`, d.id, perr.Error())
		RefundsManualReview.Inc()
		w.logger.Error("refund exhausted retries, needs manual review",
			zap.Int64("refund_id", d.id), zap.Int("attempts", d.attempts), zap.Error(perr))
		return err
	}
	backoff := w.retryBase << (d.attempts - 1)
	if backoff <= 0 || backoff > w.retryCap {
		backoff = w.retryCap
	}
	_, err := w.pool.Exec(ctx, `
		UPDATE refunds SET next_attempt_at = NOW() + make_interval(secs => $2), last_error = $3, updated_at = NOW()
		WHERE id = $1 AND status = 'pending'`, d.id, backoff.Seconds(), perr.Error())
	w.logger.Warn("refund failed, will retry",
		zap.Int64("refund_id", d.id), zap.Int("attempt", d.attempts), zap.Duration("next_in", backoff), zap.Error(perr))
	return err
}
