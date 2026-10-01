// Package payment — заглушка платёжного провайдера для PAY-01.
//
// Ведёт себя как внешняя система: свой журнал списаний (отдельная таблица,
// пишется вне наших транзакций и переживает падение нашего процесса),
// задержка ответа и случайные отказы. Отмену нашего контекста не слушает:
// запрос дошёл до провайдера — деньги списаны, даже если клиент уже ушёл.
package payment

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

var (
	ErrDeclined    = errors.New("payment declined by provider")
	ErrUnavailable = errors.New("provider unavailable")
)

type FakeProvider struct {
	pool     *pgxpool.Pool
	delay    time.Duration
	failRate float64
	logger   *zap.Logger
}

// NewFakeProvider читает PROVIDER_DELAY (по умолчанию 2s) и
// PROVIDER_FAIL_RATE (0..1, по умолчанию 0.2).
func NewFakeProvider(ctx context.Context, pool *pgxpool.Pool, logger *zap.Logger) (*FakeProvider, error) {
	delay := 2 * time.Second
	if d, err := time.ParseDuration(os.Getenv("PROVIDER_DELAY")); err == nil {
		delay = d
	}
	failRate := 0.2
	if f, err := strconv.ParseFloat(os.Getenv("PROVIDER_FAIL_RATE"), 64); err == nil {
		failRate = f
	}
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS fake_provider_charges (
			id         BIGSERIAL PRIMARY KEY,
			order_id   INT NOT NULL,
			amount     NUMERIC(12,2) NOT NULL,
			status     TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)
	if err != nil {
		return nil, fmt.Errorf("fake provider table: %w", err)
	}
	// провайдер сам дедуплицирует списания по ключу идемпотентности
	if _, err := pool.Exec(ctx, `
		ALTER TABLE fake_provider_charges ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
		CREATE UNIQUE INDEX IF NOT EXISTS fake_provider_charges_key ON fake_provider_charges (idempotency_key);`); err != nil {
		return nil, fmt.Errorf("fake provider idempotency: %w", err)
	}
	// возвраты и переключатель доступности, который можно менять на лету:
	// UPDATE fake_provider_settings SET down = true
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS fake_provider_refunds (
			id              BIGSERIAL PRIMARY KEY,
			idempotency_key TEXT UNIQUE NOT NULL,
			charge_id       TEXT NOT NULL,
			amount          NUMERIC(12,2) NOT NULL,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS fake_provider_settings (
			id   INT PRIMARY KEY,
			down BOOLEAN NOT NULL DEFAULT false
		);
		INSERT INTO fake_provider_settings (id) VALUES (1) ON CONFLICT DO NOTHING;`); err != nil {
		return nil, fmt.Errorf("fake provider refunds: %w", err)
	}
	return &FakeProvider{pool: pool, delay: delay, failRate: failRate, logger: logger}, nil
}

// Charge списывает amount. Повтор с тем же key не списывает снова,
// а возвращает результат первого вызова — как у настоящих провайдеров.
func (p *FakeProvider) Charge(_ context.Context, key string, orderID int, amount decimal.Decimal) (string, error) {
	time.Sleep(p.delay) // «сеть и банк»

	// свой контекст: на стороне провайдера наш таймаут ничего не значит
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if p.isDown(ctx) {
		return "", ErrUnavailable
	}

	status := "charged"
	if rand.Float64() < p.failRate {
		status = "declined"
	}
	var id int64
	err := p.pool.QueryRow(ctx, `
		INSERT INTO fake_provider_charges (order_id, amount, status, idempotency_key)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id`, orderID, amount, status, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// повтор: отдаём исход первой попытки
		if err := p.pool.QueryRow(ctx,
			"SELECT id, status FROM fake_provider_charges WHERE idempotency_key = $1", key).Scan(&id, &status); err != nil {
			return "", fmt.Errorf("provider internal error: %w", err)
		}
		p.logger.Info("provider: duplicate request, returning first result",
			zap.String("key", key), zap.Int64("charge_id", id), zap.String("status", status))
	} else if err != nil {
		return "", fmt.Errorf("provider internal error: %w", err)
	}
	p.logger.Info("provider", zap.Int("order_id", orderID), zap.String("status", status),
		zap.String("amount", amount.String()), zap.Int64("charge_id", id))
	if status == "declined" {
		return "", ErrDeclined
	}
	return fmt.Sprintf("ch_%d", id), nil
}

func (p *FakeProvider) isDown(ctx context.Context) bool {
	var down bool
	_ = p.pool.QueryRow(ctx, "SELECT down FROM fake_provider_settings WHERE id = 1").Scan(&down)
	return down
}

// Refund возвращает amount по списанию chargeID. Повтор с тем же key
// не возвращает деньги второй раз, а отдаёт результат первого вызова.
func (p *FakeProvider) Refund(_ context.Context, key, chargeID string, amount decimal.Decimal) (string, error) {
	time.Sleep(p.delay)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if p.isDown(ctx) {
		return "", ErrUnavailable
	}
	var id int64
	err := p.pool.QueryRow(ctx, `
		INSERT INTO fake_provider_refunds (idempotency_key, charge_id, amount)
		VALUES ($1, $2, $3)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id`, key, chargeID, amount).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := p.pool.QueryRow(ctx,
			"SELECT id FROM fake_provider_refunds WHERE idempotency_key = $1", key).Scan(&id); err != nil {
			return "", fmt.Errorf("provider internal error: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("provider internal error: %w", err)
	}
	p.logger.Info("provider: refund", zap.String("key", key), zap.String("charge_id", chargeID),
		zap.String("amount", amount.String()), zap.Int64("refund_id", id))
	return fmt.Sprintf("rf_%d", id), nil
}

// ChargeStatus — что провайдер знает о списании по ключу идемпотентности.
type ChargeStatus string

const (
	ChargeNotFound  ChargeStatus = "not_found" // запрос до провайдера не доходил
	ChargeSucceeded ChargeStatus = "charged"
	ChargeDeclined  ChargeStatus = "declined"
)

// ChargeStatus — запрос статуса списания у провайдера. Именно у провайдера,
// а не по нашим догадкам: деньги — его журнал, не наш.
func (p *FakeProvider) ChargeStatus(_ context.Context, key string) (ChargeStatus, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if p.isDown(ctx) {
		return "", "", ErrUnavailable
	}
	var id int64
	var status string
	err := p.pool.QueryRow(ctx,
		"SELECT id, status FROM fake_provider_charges WHERE idempotency_key = $1", key).Scan(&id, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChargeNotFound, "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("provider internal error: %w", err)
	}
	if status == "declined" {
		return ChargeDeclined, "", nil
	}
	return ChargeSucceeded, fmt.Sprintf("ch_%d", id), nil
}
