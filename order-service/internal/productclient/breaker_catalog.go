package productclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	"order-service/internal/breaker"
	"order-service/internal/domain"
	"order-service/internal/metrics"

	"github.com/shopspring/decimal"
)

// BreakerCatalog — предохранитель из FAIL-05 вокруг каталога. После threshold
// отказов подряд не ходим к соседу openFor: отказ мгновенный, а не через
// весь таймаут вызова, и лежащего соседа не добиваем повторами.
type BreakerCatalog struct {
	next PriceSource
	b    *breaker.Breaker
}

func NewBreakerCatalog(next PriceSource, threshold int, openFor time.Duration) *BreakerCatalog {
	metrics.BreakerState.WithLabelValues("catalog").Set(0)
	return &BreakerCatalog{
		next: next,
		b: breaker.New("catalog", threshold, openFor,
			breaker.WithOnChange(func(name string, from, to breaker.State) {
				metrics.BreakerState.WithLabelValues(name).Set(float64(to))
				metrics.BreakerTransitions.WithLabelValues(name, from.String(), to.String()).Inc()
			})),
	}
}

func (c *BreakerCatalog) Prices(ctx context.Context, ids []int) (map[int]decimal.Decimal, error) {
	if err := c.b.Allow(); err != nil {
		return nil, fmt.Errorf("%w: circuit open", domain.ErrCatalogUnavailable)
	}
	out, err := c.next.Prices(ctx, ids)
	c.b.Done(!isNeighbourFailure(err))
	return out, err
}

// Отказ соседа — только недоступность. Нехватка бюджета и отмена клиентом
// предохранитель не размыкают.
func isNeighbourFailure(err error) bool {
	return errors.Is(err, domain.ErrCatalogUnavailable) && !errors.Is(err, ErrBudgetExhausted)
}
