package reconciliation

import (
	"context"
	"fmt"
	"time"

	"order-service/internal/domain"
	"order-service/internal/payment"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

var (
	// Открытые расхождения после последнего запуска. Ноль — норма.
	Discrepancies = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "reconciliation_discrepancies",
		Help: "Discrepancies left for manual review after the last reconciliation run",
	}, []string{"kind"})
	AutoFixed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "reconciliation_auto_fixed_total",
		Help: "Discrepancies fixed automatically by reconciliation",
	}, []string{"action"})
	LastRun = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "reconciliation_last_run_timestamp_seconds",
		Help: "Unix time of the last finished reconciliation run",
	})
)

func init() { prometheus.MustRegister(Discrepancies, AutoFixed, LastRun) }

type ChargeLister interface {
	ListCharges(ctx context.Context, from, to time.Time) ([]payment.ProviderCharge, error)
}

type PaymentTransitioner interface {
	Transition(ctx context.Context, paymentID int64, to domain.PaymentStatus, providerID, reason string) error
}

type Job struct {
	pool     *pgxpool.Pool
	provider ChargeLister
	payments PaymentTransitioner
	logger   *zap.Logger
	// запас вокруг периода: запись у провайдера может лечь чуть позже нашей
	margin time.Duration
}

func NewJob(pool *pgxpool.Pool, provider ChargeLister, payments PaymentTransitioner, logger *zap.Logger) *Job {
	return &Job{pool: pool, provider: provider, payments: payments, logger: logger, margin: time.Hour}
}

type Report struct {
	RunID                               int64
	Checked, Matched, AutoFixed, Manual int
	ByKind                              map[Kind]int
}

// RunScheduled запускает сверку каждые every за окно window до текущего момента.
// Окна перекрываются: расхождение, не найденное в одном, найдётся в следующем.
func (j *Job) RunScheduled(ctx context.Context, every, window time.Duration) {
	j.logger.Info("reconciliation scheduled", zap.Duration("every", every), zap.Duration("window", window))
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			to := time.Now()
			if _, err := j.Run(ctx, to.Add(-window), to); err != nil {
				j.logger.Error("reconciliation run failed", zap.Error(err))
			}
		}
	}
}

func (j *Job) Run(ctx context.Context, from, to time.Time) (Report, error) {
	rep := Report{ByKind: map[Kind]int{}}

	theirs, err := j.provider.ListCharges(ctx, from.Add(-j.margin), to.Add(j.margin))
	if err != nil {
		return rep, fmt.Errorf("provider list: %w", err)
	}
	theirsByKey := map[string]*Theirs{}
	var providerKeys []string
	for _, c := range theirs {
		if c.Key == "" {
			continue
		}
		theirsByKey[c.Key] = &Theirs{Key: c.Key, ChargeID: c.ChargeID, Amount: c.Amount, Status: c.Status}
		if !c.CreatedAt.Before(from) && c.CreatedAt.Before(to) {
			providerKeys = append(providerKeys, c.Key)
		}
	}

	// наши платежи за период — плюс любые с ключами, которые провайдер видел
	// в периоде: наш платёж мог появиться чуть раньше границы
	rows, err := j.pool.Query(ctx, `
		SELECT p.id, p.order_id, p.idempotency_key, p.amount, p.status, COALESCE(p.provider_payment_id, ''),
		       COALESCE(r.id, 0), COALESCE(r.status, '')
		FROM payments p
		LEFT JOIN refunds r ON r.payment_id = p.id
		WHERE p.idempotency_key IS NOT NULL
		  AND ((p.created_at >= $1 AND p.created_at < $2) OR p.idempotency_key = ANY($3))`,
		from, to, providerKeys)
	if err != nil {
		return rep, err
	}
	oursByKey := map[string]*Ours{}
	for rows.Next() {
		o := &Ours{}
		var status string
		if err := rows.Scan(&o.PaymentID, &o.OrderID, &o.Key, &o.Amount, &status, &o.ChargeID,
			&o.RefundID, &o.RefundStatus); err != nil {
			rows.Close()
			return rep, err
		}
		o.Status = domain.PaymentStatus(status)
		oursByKey[o.Key] = o
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return rep, err
	}

	keys := map[string]bool{}
	for k := range oursByKey {
		keys[k] = true
	}
	for _, k := range providerKeys {
		keys[k] = true
	}

	if err := j.pool.QueryRow(ctx,
		"INSERT INTO reconciliation_runs (period_from, period_to) VALUES ($1, $2) RETURNING id",
		from, to).Scan(&rep.RunID); err != nil {
		return rep, err
	}

	for k := range keys {
		ours, th := oursByKey[k], theirsByKey[k]
		f := Classify(ours, th)
		rep.Checked++
		if f.Kind == Matched {
			rep.Matched++
			continue
		}
		action := "manual"
		if f.Action != Manual {
			if err := j.fix(ctx, f.Action, ours, th); err != nil {
				f.Note += "; auto-fix failed: " + err.Error()
			} else {
				action = "auto_fixed"
				rep.AutoFixed++
				AutoFixed.WithLabelValues(string(f.Action)).Inc()
			}
		}
		if action == "manual" {
			rep.Manual++
			rep.ByKind[f.Kind]++
		}
		if err := j.record(ctx, rep.RunID, k, f, action, ours, th); err != nil {
			return rep, err
		}
	}

	if _, err := j.pool.Exec(ctx, `
		UPDATE reconciliation_runs
		SET finished_at = NOW(), checked = $2, matched = $3, auto_fixed = $4, manual = $5
		WHERE id = $1`, rep.RunID, rep.Checked, rep.Matched, rep.AutoFixed, rep.Manual); err != nil {
		return rep, err
	}

	for _, k := range []Kind{MissingAtProvider, MissingAtOurs, AmountMismatch, StatusMismatch} {
		Discrepancies.WithLabelValues(string(k)).Set(float64(rep.ByKind[k]))
	}
	LastRun.SetToCurrentTime()
	j.logger.Info("reconciliation finished", zap.Int64("run_id", rep.RunID), zap.Int("checked", rep.Checked),
		zap.Int("matched", rep.Matched), zap.Int("auto_fixed", rep.AutoFixed), zap.Int("manual", rep.Manual))
	return rep, nil
}

func (j *Job) fix(ctx context.Context, a Action, ours *Ours, th *Theirs) error {
	reason := "reconciliation: provider says " + th.Status
	switch a {
	case FixPaymentSucceeded:
		return j.payments.Transition(ctx, ours.PaymentID, domain.PaymentSucceeded, th.ChargeID, reason)
	case FixPaymentFailed:
		return j.payments.Transition(ctx, ours.PaymentID, domain.PaymentFailed, "", reason)
	case FixRefundSucceeded:
		if ours.Status == domain.PaymentSucceeded {
			if err := j.payments.Transition(ctx, ours.PaymentID, domain.PaymentRefunded, "", reason); err != nil {
				return err
			}
		}
		_, err := j.pool.Exec(ctx, `
			UPDATE refunds SET status = 'succeeded', updated_at = NOW(),
			       last_error = 'confirmed by reconciliation'
			WHERE id = $1 AND status IN ('pending', 'manual_review')`, ours.RefundID)
		return err
	}
	return fmt.Errorf("unknown action %q", a)
}

func (j *Job) record(ctx context.Context, runID int64, key string, f Finding, action string, ours *Ours, th *Theirs) error {
	var paymentID, orderID any
	var ourStatus, ourAmount any
	if ours != nil {
		paymentID, orderID, ourStatus, ourAmount = ours.PaymentID, ours.OrderID, string(ours.Status), ours.Amount
	}
	var thStatus, thAmount, thCharge any
	if th != nil {
		thStatus, thAmount, thCharge = th.Status, th.Amount, th.ChargeID
	}
	_, err := j.pool.Exec(ctx, `
		INSERT INTO reconciliation_items
		  (run_id, kind, action, idempotency_key, payment_id, order_id,
		   our_status, our_amount, provider_status, provider_amount, provider_charge_id, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		runID, string(f.Kind), action, key, paymentID, orderID, ourStatus, ourAmount, thStatus, thAmount, thCharge, f.Note)
	return err
}
