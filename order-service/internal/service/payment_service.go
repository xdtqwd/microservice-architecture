package service

import (
	"context"
	"errors"
	"fmt"
	"os"

	"order-service/internal/domain"
	"order-service/internal/payment"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

type PayRepository interface {
	GetOrderByID(ctx context.Context, id int) (*domain.Order, error)
	MarkPaid(ctx context.Context, id int) error
}

type PaymentProvider interface {
	Charge(ctx context.Context, orderID int, amount decimal.Decimal) (string, error)
}

// PaymentService — НАИВНАЯ оплата из PAY-01. Ломается намеренно:
// проверка статуса и запись разнесены, идемпотентности нет.
type PaymentService struct {
	repo     PayRepository
	provider PaymentProvider
	logger   *zap.Logger

	// демонстрационные переключатели PAY-01, удалить в PAY-02
	writeFirst       bool // PAY_WRITE_FIRST=1: сначала paid в базу, потом провайдер
	crashAfterCharge bool // PAY_CRASH_AFTER_CHARGE=1: падаем между провайдером и базой
}

func NewPaymentService(repo PayRepository, provider PaymentProvider, logger *zap.Logger) *PaymentService {
	return &PaymentService{
		repo: repo, provider: provider, logger: logger,
		writeFirst:       os.Getenv("PAY_WRITE_FIRST") == "1",
		crashAfterCharge: os.Getenv("PAY_CRASH_AFTER_CHARGE") == "1",
	}
}

func (s *PaymentService) Pay(ctx context.Context, orderID int) (string, error) {
	order, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return "", err
	}
	if order.Status != "pending" {
		return "", fmt.Errorf("%w: order is %s", domain.ErrInvalidStatusTransition, order.Status)
	}

	amount := decimal.Zero
	for _, it := range order.Items {
		amount = amount.Add(it.Price.Mul(decimal.NewFromInt(int64(it.Quantity))))
	}

	if s.writeFirst {
		if err := s.repo.MarkPaid(ctx, orderID); err != nil {
			return "", err
		}
		chargeID, err := s.provider.Charge(ctx, orderID, amount)
		if err != nil {
			return "", s.mapErr(err) // заказ уже paid — наивно не откатываем
		}
		return chargeID, nil
	}

	chargeID, err := s.provider.Charge(ctx, orderID, amount)
	if err != nil {
		return "", s.mapErr(err)
	}
	if s.crashAfterCharge {
		s.logger.Error("PAY-01 demo: crashing after charge, before DB write",
			zap.Int("order_id", orderID), zap.String("charge_id", chargeID))
		os.Exit(1)
	}
	if err := s.repo.MarkPaid(ctx, orderID); err != nil {
		return "", err
	}
	return chargeID, nil
}

func (s *PaymentService) mapErr(err error) error {
	if errors.Is(err, payment.ErrDeclined) {
		return fmt.Errorf("%w: %v", domain.ErrPaymentDeclined, err)
	}
	return err
}
