package service

import (
	"context"
	"errors"
	"fmt"
	"os"

	"order-service/internal/domain"
	"order-service/internal/payment"
	"order-service/internal/txm"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

type PaymentStore interface {
	Start(ctx context.Context, orderID int) (int64, decimal.Decimal, error)
	Transition(ctx context.Context, paymentID int64, to domain.PaymentStatus, providerID, reason string) error
}

type PaymentProvider interface {
	Charge(ctx context.Context, orderID int, amount decimal.Decimal) (string, error)
}

type PaymentService struct {
	store    PaymentStore
	provider PaymentProvider
	logger   *zap.Logger

	// демо: падение между ответом провайдера и записью результата.
	// Нужно, чтобы показать зависший pending-платёж и сверку в PAY-04.
	crashAfterCharge bool
}

func NewPaymentService(store PaymentStore, provider PaymentProvider, logger *zap.Logger) *PaymentService {
	return &PaymentService{
		store: store, provider: provider, logger: logger,
		crashAfterCharge: os.Getenv("PAY_CRASH_AFTER_CHARGE") == "1",
	}
}

func (s *PaymentService) Pay(ctx context.Context, orderID int) (string, error) {
	paymentID, amount, err := s.store.Start(ctx, orderID)
	if err != nil {
		return "", err
	}

	chargeID, err := s.provider.Charge(ctx, orderID, amount)

	// Результат внешнего действия записываем независимо от клиента:
	// если он отключился, деньги всё равно списаны или отклонены.
	rctx, cancel := txm.Detached()
	defer cancel()

	switch {
	case errors.Is(err, payment.ErrDeclined):
		if terr := s.store.Transition(rctx, paymentID, domain.PaymentFailed, "", err.Error()); terr != nil {
			s.logger.Error("failed to record declined payment", zap.Int64("payment_id", paymentID), zap.Error(terr))
		}
		return "", fmt.Errorf("%w: %v", domain.ErrPaymentDeclined, err)
	case err != nil:
		// исход неизвестен: платёж остаётся pending до сверки с провайдером
		s.logger.Warn("payment outcome unknown, left pending",
			zap.Int64("payment_id", paymentID), zap.Error(err))
		return "", err
	}

	if s.crashAfterCharge {
		s.logger.Error("demo: crashing after charge, before recording result",
			zap.Int64("payment_id", paymentID), zap.String("charge_id", chargeID))
		os.Exit(1)
	}

	if err := s.store.Transition(rctx, paymentID, domain.PaymentSucceeded, chargeID, "provider confirmed"); err != nil {
		s.logger.Error("charged but failed to record success",
			zap.Int64("payment_id", paymentID), zap.String("charge_id", chargeID), zap.Error(err))
		return "", err
	}
	return chargeID, nil
}
