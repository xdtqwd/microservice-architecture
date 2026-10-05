package productclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	"order-service/internal/domain"
	"order-service/internal/gen/productv1"
	"order-service/internal/metrics"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrBudgetExhausted — на вызов соседа не осталось времени. Это не отказ
// соседа, предохранитель его не считает.
var ErrBudgetExhausted = errors.New("request budget exhausted")

// GRPCCatalog — цены из product-service, одним вызовом на весь заказ.
type GRPCCatalog struct {
	client  productv1.ProductServiceClient
	timeout time.Duration
	// reserve — сколько времени запросу нужно после ответа соседа:
	// транзакция заказа и ответ клиенту. Его соседу не отдаём.
	reserve time.Duration
	// minCall — если на вызов остаётся меньше, не звоним вовсе:
	// заведомо не успеем, только нагрузим соседа.
	minCall time.Duration
}

func NewGRPCCatalog(conn grpc.ClientConnInterface, timeout time.Duration) *GRPCCatalog {
	return &GRPCCatalog{
		client:  productv1.NewProductServiceClient(conn),
		timeout: timeout,
		reserve: 300 * time.Millisecond,
		minCall: 20 * time.Millisecond,
	}
}

// callTimeout — бюджет на вызов соседа: не больше своего таймаута
// и не больше того, что осталось у запроса за вычетом резерва.
func (c *GRPCCatalog) callTimeout(ctx context.Context) (time.Duration, error) {
	t := c.timeout
	if dl, ok := ctx.Deadline(); ok {
		left := time.Until(dl) - c.reserve
		if left < c.minCall {
			metrics.RPCClientSkipped.WithLabelValues("GetProducts").Inc()
			return 0, fmt.Errorf("%w: %w (%s left)", domain.ErrCatalogUnavailable, ErrBudgetExhausted,
				time.Until(dl).Round(time.Millisecond))
		}
		if left < t {
			t = left
		}
	}
	return t, nil
}

// Prices возвращает цены найденных товаров. Отсутствующих в ответе нет —
// решать, ошибка ли это, вызывающему.
func (c *GRPCCatalog) Prices(ctx context.Context, ids []int) (map[int]decimal.Decimal, error) {
	req := &productv1.GetProductsRequest{Ids: uniq(ids)}
	if len(req.Ids) == 0 {
		return map[int]decimal.Decimal{}, nil
	}

	// Соседу — часть бюджета запроса, а не весь: иначе медленный сосед
	// съест время на всё остальное. Дедлайн уедет на сервер в grpc-timeout.
	timeout, err := c.callTimeout(ctx)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := c.client.GetProducts(cctx, req)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, ctx.Err() // клиент ушёл сам, сосед не виноват
		}
		switch status.Code(err) {
		case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
			return nil, fmt.Errorf("%w: %v", domain.ErrCatalogUnavailable, err)
		}
		return nil, fmt.Errorf("catalog: %w", err)
	}

	out := make(map[int]decimal.Decimal, len(resp.Products))
	for _, p := range resp.Products {
		if p.GetPrice().GetCurrencyCode() != "RUB" {
			return nil, fmt.Errorf("catalog: product %d priced in %q, expected RUB", p.Id, p.GetPrice().GetCurrencyCode())
		}
		price, err := MoneyFromProto(p.Price)
		if err != nil {
			return nil, fmt.Errorf("catalog: product %d: %w", p.Id, err)
		}
		out[int(p.Id)] = price
	}
	return out, nil
}

func uniq(ids []int) []int64 {
	seen := make(map[int]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, int64(id))
		}
	}
	return out
}
