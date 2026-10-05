package productclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	"order-service/internal/domain"
	"order-service/internal/gen/productv1"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCCatalog — цены из product-service, одним вызовом на весь заказ.
type GRPCCatalog struct {
	client  productv1.ProductServiceClient
	timeout time.Duration
}

func NewGRPCCatalog(conn grpc.ClientConnInterface, timeout time.Duration) *GRPCCatalog {
	return &GRPCCatalog{client: productv1.NewProductServiceClient(conn), timeout: timeout}
}

// Prices возвращает цены найденных товаров. Отсутствующих в ответе нет —
// решать, ошибка ли это, вызывающему.
func (c *GRPCCatalog) Prices(ctx context.Context, ids []int) (map[int]decimal.Decimal, error) {
	req := &productv1.GetProductsRequest{Ids: uniq(ids)}
	if len(req.Ids) == 0 {
		return map[int]decimal.Decimal{}, nil
	}

	// Свой короткий таймаут: соседу отводим часть бюджета запроса (10s),
	// а не весь — иначе один медленный сосед съест время на всё остальное.
	cctx, cancel := context.WithTimeout(ctx, c.timeout)
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
