package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"order-service/internal/domain"
	"order-service/internal/payment"
	"order-service/internal/txm"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

var (
	// Зависшие платежи: pending дольше stuckAfter. Видно раньше, чем придёт клиент.
	PaymentsStuck = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "payments_stuck",
		Help: "Pending payments older than the stuck threshold",
	})
	PaymentsReconciled = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "payments_reconciled_total",
		Help: "Stuck payments resolved by reconciliation, by outcome",
	}, []string{"outcome"})
)

func init() { prometheus.MustRegister(PaymentsStuck, PaymentsReconciled) }

type ChargeChecker interface {
	ChargeStatus(ctx context.Context, key string) (payment.ChargeStatus, string, error)
}

type PaymentTransitioner interface {
	Transition(ctx context.Context, paymentID int64, to domain.PaymentStatus, providerID, reason string) error
}

type ReconcileWorker struct {
	pool     *pgxpool.Pool
	checker  ChargeChecker
	payments PaymentTransitioner
	logger   *zap.Logger

	interval    time.Duration
	lease       time.Duration
	stuckAfter  time.Duration // pending дольше этого — зависший
	expireAfter time.Duration // провайдер так и не узнал ключ — запрос не дошёл
	retryBase   time.Duration
	retryCap    time.Duration
	maxAttempts int
	batch       int
}

type ReconcileOption func(*ReconcileWorker)

func WithReconcileTiming(stuckAfter, expireAfter, retryBase, retryCap time.Duration, maxAttempts int) ReconcileOption {
	return func(w *ReconcileWorker) {
		w.stuckAfter, w.expireAfter = stuckAfter, expireAfter
		w.retryBase, w.retryCap, w.maxAttempts = retryBase, retryCap, maxAttempts
	}
}

func NewReconcileWorker(pool *pgxpool.Pool, checker ChargeChecker, payments PaymentTransitioner,
	logger *zap.Logger, opts ...ReconcileOption) *ReconcileWorker {
	w := &ReconcileWorker{
		pool: pool, checker: checker, payments: payments, logger: logger,
		interval: 5 * time.Second,
		lease:    30 * time.Second,
		// Запрос на оплату живёт максимум 10s (дедлайн) — после 2 минут
		// это уже точно не идущий запрос, а зависший.
		stuckAfter: 2 * time.Minute,
		// Если за 15 минут провайдер так и не узнал ключ — запрос до него
		// не дошёл, и уже не дойдёт.
		expireAfter: 15 * time.Minute,
		retryBase:   10 * time.Second,
		retryCap:    5 * time.Minute,
		maxAttempts: 10,
		batch:       20,
	}
	for _, o := range opts {
		o(w)
	}
	return w
}

func (w *ReconcileWorker) Run(ctx context.Context) {
	w.logger.Info("payment reconcile worker started",
		zap.Duration("stuck_after", w.stuckAfter), zap.Duration("expire_after", w.expireAfter))
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("payment reconcile worker stopped")
			return
		case <-t.C:
			if err := w.ProcessDue(ctx); err != nil {
				w.logger.Warn("reconcile worker", zap.Error(err))
			}
		}
	}
}

type stuckPayment struct {
	id       int64
	key      string
	attempts int
	age      time.Duration
}

func (w *ReconcileWorker) ProcessDue(ctx context.Context) error {
	var stuck int
	if err := w.pool.QueryRow(ctx, `
		SELECT count(*) FROM payments
		WHERE status = 'pending' AND created_at < NOW() - make_interval(secs => $1)`,
		w.stuckAfter.Seconds()).Scan(&stuck); err == nil {
		PaymentsStuck.Set(float64(stuck))
	}

	// аренда пачки — как в релее outbox: SKIP LOCKED, реплики не мешают друг другу
	rows, err := w.pool.Query(ctx, `
		UPDATE payments
		SET reconcile_attempts = reconcile_attempts + 1,
		    next_reconcile_at = NOW() + make_interval(secs => $3)
		WHERE id IN (
		    SELECT id FROM payments
		    WHERE status = 'pending' AND manual_review_reason IS NULL
		      AND created_at < NOW() - make_interval(secs => $1)
		      AND next_reconcile_at <= NOW()
		    ORDER BY id LIMIT $2
		    FOR UPDATE SKIP LOCKED)
		RETURNING id, COALESCE(idempotency_key, ''), reconcile_attempts,
		          EXTRACT(EPOCH FROM NOW() - created_at)::float8`,
		w.stuckAfter.Seconds(), w.batch, w.lease.Seconds())
	if err != nil {
		return err
	}
	var batch []stuckPayment
	for rows.Next() {
		var p stuckPayment
		var ageSec float64
		if err := rows.Scan(&p.id, &p.key, &p.attempts, &ageSec); err != nil {
			rows.Close()
			return err
		}
		p.age = time.Duration(ageSec * float64(time.Second))
		batch = append(batch, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, p := range batch {
		w.reconcile(ctx, p)
	}
	return nil
}

func (w *ReconcileWorker) reconcile(ctx context.Context, p stuckPayment) {
	if p.key == "" {
		w.manual(p, "no idempotency key: cannot ask provider about this payment")
		return
	}

	status, chargeID, err := w.checker.ChargeStatus(ctx, p.key)

	rctx, cancel := txm.Detached()
	defer cancel()

	switch {
	case err != nil:
		w.retryOrManual(rctx, p, "provider status check failed: "+err.Error())
	case status == payment.ChargeSucceeded:
		w.resolve(rctx, p, domain.PaymentSucceeded, chargeID, "reconciled: charged at provider", "succeeded")
	case status == payment.ChargeDeclined:
		w.resolve(rctx, p, domain.PaymentFailed, "", "reconciled: declined at provider", "failed")
	case status == payment.ChargeNotFound && p.age >= w.expireAfter:
		w.resolve(rctx, p, domain.PaymentExpired, "",
			fmt.Sprintf("reconciled: provider has no charge after %s", p.age.Round(time.Second)), "expired")
	default:
		// провайдер ключа пока не знает, а платёж ещё молод — запрос мог не дойти, ждём
		w.schedule(rctx, p, w.backoff(p.attempts), "provider has not seen this key yet")
	}
}

func (w *ReconcileWorker) resolve(ctx context.Context, p stuckPayment, to domain.PaymentStatus, providerID, reason, outcome string) {
	err := w.payments.Transition(ctx, p.id, to, providerID, reason)
	if errors.Is(err, domain.ErrPaymentTransition) {
		// уже разрешён параллельно — например, клиент повторил запрос
		w.logger.Info("payment already resolved", zap.Int64("payment_id", p.id))
		return
	}
	if err != nil {
		w.retryOrManual(ctx, p, "record result: "+err.Error())
		return
	}
	PaymentsReconciled.WithLabelValues(outcome).Inc()
	w.logger.Info("stuck payment reconciled", zap.Int64("payment_id", p.id),
		zap.String("outcome", outcome), zap.Duration("age", p.age))
}

func (w *ReconcileWorker) retryOrManual(ctx context.Context, p stuckPayment, reason string) {
	if p.attempts >= w.maxAttempts {
		w.manual(p, fmt.Sprintf("gave up after %d attempts: %s", p.attempts, reason))
		return
	}
	w.schedule(ctx, p, w.backoff(p.attempts), reason)
}

func (w *ReconcileWorker) schedule(ctx context.Context, p stuckPayment, after time.Duration, reason string) {
	if _, err := w.pool.Exec(ctx,
		"UPDATE payments SET next_reconcile_at = NOW() + make_interval(secs => $2) WHERE id = $1 AND status = 'pending'",
		p.id, after.Seconds()); err != nil {
		w.logger.Error("reconcile: schedule failed", zap.Int64("payment_id", p.id), zap.Error(err))
	}
	w.logger.Info("stuck payment: will check again", zap.Int64("payment_id", p.id),
		zap.Int("attempt", p.attempts), zap.Duration("next_in", after), zap.String("reason", reason))
}

func (w *ReconcileWorker) manual(p stuckPayment, reason string) {
	ctx, cancel := txm.Detached()
	defer cancel()
	if _, err := w.pool.Exec(ctx,
		"UPDATE payments SET manual_review_reason = $2 WHERE id = $1 AND status = 'pending'", p.id, reason); err != nil {
		w.logger.Error("reconcile: manual review mark failed", zap.Int64("payment_id", p.id), zap.Error(err))
		return
	}
	PaymentsReconciled.WithLabelValues("manual_review").Inc()
	w.logger.Error("stuck payment needs manual review", zap.Int64("payment_id", p.id), zap.String("reason", reason))
}

func (w *ReconcileWorker) backoff(attempts int) time.Duration {
	d := w.retryBase << (attempts - 1)
	if d <= 0 || d > w.retryCap {
		return w.retryCap
	}
	return d
}
