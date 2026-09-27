package service

import (
	"context"
	"errors"
	"order-service/internal/domain"
	"order-service/internal/kafka"
	"order-service/internal/repository"
	"order-service/internal/retry"
	"order-service/internal/txm"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

const (
	defaultLimit = 50
	maxLimit     = 100
)

type OrderService struct {
	repo     OrderRepository
	txm      *txm.TxManager
	logger   *zap.Logger
	producer *kafka.Producer
	outbox   *repository.OutboxRepo
	group    singleflight.Group
}

func NewOrderService(repo OrderRepository, txm *txm.TxManager, logger *zap.Logger, producer *kafka.Producer, outbox *repository.OutboxRepo) *OrderService {
	return &OrderService{repo: repo, txm: txm, logger: logger, producer: producer, outbox: outbox}
}

// createLeaderTimeout — предел для лидера singleflight при создании заказа:
// захват соединения (1s) + транзакция с ретраями. Меньше дедлайна запроса (10s).
const createLeaderTimeout = 5 * time.Second

func (s *OrderService) CreateOrder(ctx context.Context, items []domain.CreateOrderItem, idempotencyKey string) (int, bool, error) {
	if idempotencyKey != "" {
		type result struct {
			id     int
			exists bool
		}
		// Лидер работает на Background, чтобы отмена одного клиента не убила
		// создание заказа для всех, кто пришёл с тем же ключом. Но с пределом:
		// «навсегда» держать транзакцию и соединение нельзя.
		// DoChan + select: каждый ждущий уходит по своему дедлайну,
		// а не висит, пока лидер не закончит.
		ch := s.group.DoChan(idempotencyKey, func() (interface{}, error) {
			leaderCtx, cancel := context.WithTimeout(context.Background(), createLeaderTimeout)
			defer cancel()
			id, exists, err := s.createOrder(leaderCtx, items, idempotencyKey)
			return result{id, exists}, err
		})
		select {
		case <-ctx.Done():
			return 0, false, ctx.Err()
		case res := <-ch:
			if res.Err != nil {
				return 0, false, res.Err
			}
			r := res.Val.(result)
			return r.id, r.exists, nil
		}
	}
	return s.createOrder(ctx, items, "")
}

func (s *OrderService) createOrder(ctx context.Context, items []domain.CreateOrderItem, idempotencyKey string) (int, bool, error) {
	if len(items) == 0 {
		return 0, false, errors.New("order must have at least one item")
	}

	seen := make(map[int]bool)
	var orderItems []domain.OrderItem

	for _, item := range items {
		if item.Quantity <= 0 {
			return 0, false, errors.New("quantity must be greater than 0")
		}
		if seen[item.ProductID] {
			return 0, false, errors.New("duplicate product_id")
		}
		seen[item.ProductID] = true

		orderItems = append(orderItems, domain.OrderItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		})
	}

	var orderID int
	var exists bool
	retries := 0
	err := retry.Do(ctx, s.logger, &retries, func(ctx context.Context) error {
		if s.txm == nil {
			var e error
			orderID, exists, e = s.repo.CreateOrder(ctx, orderItems, idempotencyKey)
			return e
		}
		return s.txm.Do(ctx, func(ctx context.Context) error {
			var e error
			orderID, exists, e = s.repo.CreateOrder(ctx, orderItems, idempotencyKey)
			if e != nil || exists {
				return e
			}
			if s.outbox != nil {
				e = s.outbox.Insert(ctx, orderID, "order_created", map[string]int{"order_id": orderID})
			}
			return e
		})
	})
	if err == nil && !exists && s.producer != nil {
		// СЦЕНАРИЙ 1: раскомментировать чтобы воспроизвести
		// СЦЕНАРИЙ 1: раскомментировать чтобы воспроизвести
		// panic("process killed after commit, before kafka send")
		// НАИВНЫЙ ВАРИАНТ: отправка после коммита
		// Если процесс упадёт здесь — заказ есть в БД, события нет
		if kafkaErr := s.producer.SendOrderCreated(ctx, orderID); kafkaErr != nil {
			s.logger.Error("failed to send order_created event", zap.Int("order_id", orderID), zap.Error(kafkaErr))
		}
	}
	return orderID, exists, err
}

func (s *OrderService) GetOrders(ctx context.Context, limit int, cursor *domain.OrderCursor) ([]domain.Order, *domain.OrderCursor, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return s.repo.GetOrders(ctx, limit, cursor)
}

func (s *OrderService) GetOrderByID(ctx context.Context, id int) (*domain.Order, error) {
	return s.repo.GetOrderByID(ctx, id)
}

func (s *OrderService) CancelOrder(ctx context.Context, id int) (int, error) {
	order, err := s.repo.GetOrderByID(ctx, id)
	if err != nil {
		return 0, err
	}
	if order.Status == "cancelled" {
		return 0, domain.ErrOrderAlreadyCancelled
	}
	return s.repo.CancelOrder(ctx, id)
}
