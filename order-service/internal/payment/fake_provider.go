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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

var ErrDeclined = errors.New("payment declined by provider")

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
	return &FakeProvider{pool: pool, delay: delay, failRate: failRate, logger: logger}, nil
}

// Charge списывает amount. Возвращает id списания.
func (p *FakeProvider) Charge(_ context.Context, orderID int, amount decimal.Decimal) (string, error) {
	time.Sleep(p.delay) // «сеть и банк»

	// свой контекст: на стороне провайдера наш таймаут ничего не значит
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status := "charged"
	if rand.Float64() < p.failRate {
		status = "declined"
	}
	var id int64
	err := p.pool.QueryRow(ctx,
		"INSERT INTO fake_provider_charges (order_id, amount, status) VALUES ($1, $2, $3) RETURNING id",
		orderID, amount, status).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("provider internal error: %w", err)
	}
	p.logger.Info("provider", zap.Int("order_id", orderID), zap.String("status", status),
		zap.String("amount", amount.String()), zap.Int64("charge_id", id))
	if status == "declined" {
		return "", ErrDeclined
	}
	return fmt.Sprintf("ch_%d", id), nil
}
