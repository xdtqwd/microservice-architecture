package repository

import (
	"context"
	"errors"
	"fmt"

	"order-service/internal/domain"
	"order-service/internal/txm"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

type PaymentRepo struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

func NewPaymentRepo(pool *pgxpool.Pool, logger *zap.Logger) *PaymentRepo {
	return &PaymentRepo{pool: pool, logger: logger}
}

func rollbackDetached(tx pgx.Tx) {
	ctx, cancel := txm.Detached()
	defer cancel()
	_ = tx.Rollback(ctx)
}

// PaymentInfo — состояние платежа, найденного или созданного по ключу.
type PaymentInfo struct {
	ID                int64
	Amount            decimal.Decimal
	Status            domain.PaymentStatus
	ProviderPaymentID string
}

// Start создаёт платёж без ключа идемпотентности.
func (r *PaymentRepo) Start(ctx context.Context, orderID int) (int64, decimal.Decimal, error) {
	p, err := r.StartOrResume(ctx, orderID, "")
	return p.ID, p.Amount, err
}

func (r *PaymentRepo) byKey(ctx context.Context, key string) (PaymentInfo, error) {
	var p PaymentInfo
	var status string
	err := r.pool.QueryRow(ctx, `
		SELECT id, amount, status, COALESCE(provider_payment_id, '')
		FROM payments WHERE idempotency_key = $1`, key).Scan(&p.ID, &p.Amount, &status, &p.ProviderPaymentID)
	p.Status = domain.PaymentStatus(status)
	return p, err
}

// StartOrResume возвращает платёж с этим ключом, если он уже есть
// (повтор после сбоя), иначе создаёт новый в pending.
// Второй активный платёж на заказ отбивает уникальный индекс, а не проверка в коде.
func (r *PaymentRepo) StartOrResume(ctx context.Context, orderID int, key string) (PaymentInfo, error) {
	if key != "" {
		p, err := r.byKey(ctx, key)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return PaymentInfo{}, err
		}
	}
	id, amount, err := r.start(ctx, orderID, key)
	if isUniqueViolation(err, "payments_idempotency_key") {
		return r.byKey(ctx, key) // параллельный запрос с тем же ключом успел первым
	}
	return PaymentInfo{ID: id, Amount: amount, Status: domain.PaymentPending}, err
}

func (r *PaymentRepo) start(ctx context.Context, orderID int, key string) (int64, decimal.Decimal, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, decimal.Zero, err
	}
	defer rollbackDetached(tx)

	var orderStatus string
	err = tx.QueryRow(ctx, "SELECT status FROM orders WHERE id = $1 FOR UPDATE", orderID).Scan(&orderStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, decimal.Zero, domain.ErrOrderNotFound
	}
	if err != nil {
		return 0, decimal.Zero, err
	}
	if orderStatus != "pending" {
		return 0, decimal.Zero, fmt.Errorf("%w: order is %s", domain.ErrInvalidStatusTransition, orderStatus)
	}

	var amount decimal.Decimal
	if err := tx.QueryRow(ctx,
		"SELECT COALESCE(SUM(price * quantity), 0) FROM order_items WHERE order_id = $1",
		orderID).Scan(&amount); err != nil {
		return 0, decimal.Zero, err
	}

	var id int64
	err = tx.QueryRow(ctx,
		"INSERT INTO payments (order_id, amount, status, idempotency_key) VALUES ($1, $2, 'pending', NULLIF($3, '')) RETURNING id",
		orderID, amount, key).Scan(&id)
	if isUniqueViolation(err, "payments_one_active_per_order") {
		return 0, decimal.Zero, domain.ErrPaymentAlreadyActive
	}
	if isUniqueViolation(err, "payments_idempotency_key") {
		return 0, decimal.Zero, err
	}
	if err != nil {
		return 0, decimal.Zero, err
	}
	if err := addPaymentEvent(ctx, tx, id, "", domain.PaymentPending, "started"); err != nil {
		return 0, decimal.Zero, err
	}
	return id, amount, commitDetached(tx)
}

// Transition переводит платёж в новое состояние. Недопустимый переход —
// явная ошибка. При succeeded заказ становится paid в той же транзакции.
func (r *PaymentRepo) Transition(ctx context.Context, paymentID int64, to domain.PaymentStatus, providerID, reason string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackDetached(tx)

	var fromStr string
	var orderID int
	err = tx.QueryRow(ctx,
		"SELECT status, order_id FROM payments WHERE id = $1 FOR UPDATE", paymentID).Scan(&fromStr, &orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrPaymentNotFound
	}
	if err != nil {
		return err
	}
	from := domain.PaymentStatus(fromStr)
	if !domain.CanTransitionPayment(from, to) {
		return fmt.Errorf("%w: %s -> %s", domain.ErrPaymentTransition, from, to)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE payments
		SET status = $1, provider_payment_id = COALESCE(NULLIF($2, ''), provider_payment_id), updated_at = NOW()
		WHERE id = $3`, string(to), providerID, paymentID); err != nil {
		return err
	}
	if err := addPaymentEvent(ctx, tx, paymentID, from, to, reason); err != nil {
		return err
	}

	if to == domain.PaymentSucceeded {
		tag, err := tx.Exec(ctx,
			"UPDATE orders SET status = 'paid' WHERE id = $1 AND status = 'pending'", orderID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("%w: order %d is no longer pending", domain.ErrInvalidStatusTransition, orderID)
		}
	}
	return commitDetached(tx)
}

func addPaymentEvent(ctx context.Context, tx pgx.Tx, paymentID int64, from, to domain.PaymentStatus, reason string) error {
	var fromVal any
	if from != "" {
		fromVal = string(from)
	}
	_, err := tx.Exec(ctx,
		"INSERT INTO payment_events (payment_id, from_status, to_status, reason) VALUES ($1, $2, $3, $4)",
		paymentID, fromVal, string(to), reason)
	return err
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
