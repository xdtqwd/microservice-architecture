package productclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"order-service/internal/domain"
	"order-service/internal/gen/productv1"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// ---------- устаревший кеш ----------

type switchSource struct {
	calls atomic.Int64
	err   error
	delay time.Duration
}

func (s *switchSource) Prices(_ context.Context, ids []int) (map[int]decimal.Decimal, error) {
	s.calls.Add(1)
	time.Sleep(s.delay)
	if s.err != nil {
		return nil, s.err
	}
	out := map[int]decimal.Decimal{}
	for _, id := range ids {
		out[id] = decimal.NewFromInt(int64(id * 100))
	}
	return out, nil
}

var errDown = fmt.Errorf("%w: neighbour down", domain.ErrCatalogUnavailable)

func TestStale_ServedWhileNeighbourDown_WithinLimit(t *testing.T) {
	src := &switchSource{}
	c := NewCachedCatalog(src, 100, 30*time.Second).WithMaxStale(10 * time.Minute)
	t0 := time.Now()
	c.now = func() time.Time { return t0 }
	_, err := c.Prices(context.Background(), []int{1})
	require.NoError(t, err)

	src.err = errDown
	c.now = func() time.Time { return t0.Add(5 * time.Minute) } // TTL прошёл, предел — нет

	prices, err := c.Prices(context.Background(), []int{1})
	require.NoError(t, err, "сосед лежит, но цена не старше предела — продаём")
	assert.True(t, decimal.NewFromInt(100).Equal(prices[1]))

	_, err = c.Prices(context.Background(), []int{2})
	assert.ErrorIs(t, err, domain.ErrCatalogUnavailable, "товара не было в кеше — честный отказ")

	_, err = c.Prices(context.Background(), []int{1, 2})
	assert.ErrorIs(t, err, domain.ErrCatalogUnavailable, "нет цены хотя бы на один товар — отказ всему заказу")

	c.now = func() time.Time { return t0.Add(11 * time.Minute) }
	_, err = c.Prices(context.Background(), []int{1})
	assert.ErrorIs(t, err, domain.ErrCatalogUnavailable, "старше предела — не продаём")
}

func TestStale_NotUsedForOtherErrors(t *testing.T) {
	src := &switchSource{}
	c := NewCachedCatalog(src, 100, 30*time.Second).WithMaxStale(10 * time.Minute)
	t0 := time.Now()
	c.now = func() time.Time { return t0 }
	_, _ = c.Prices(context.Background(), []int{1})

	src.err = errors.New("corrupted response") // не недоступность, а что-то сломано
	c.now = func() time.Time { return t0.Add(time.Minute) }
	_, err := c.Prices(context.Background(), []int{1})
	assert.Error(t, err, "устаревшую цену отдаём только когда сосед недоступен")
}

// ---------- предохранитель ----------

func TestBreaker_OpensAfterFailures_ThenFailsFast(t *testing.T) {
	src := &switchSource{err: errDown, delay: 50 * time.Millisecond}
	c := NewBreakerCatalog(src, 3, time.Minute)

	for i := 0; i < 3; i++ {
		_, err := c.Prices(context.Background(), []int{1})
		require.ErrorIs(t, err, domain.ErrCatalogUnavailable)
	}

	start := time.Now()
	_, err := c.Prices(context.Background(), []int{1})

	assert.ErrorIs(t, err, domain.ErrCatalogUnavailable)
	assert.Less(t, time.Since(start), 10*time.Millisecond, "разомкнут — отказ мгновенный, без ожидания таймаута")
	assert.EqualValues(t, 3, src.calls.Load(), "к лежащему соседу больше не ходим")
}

func TestBreaker_BudgetExhaustion_DoesNotOpen(t *testing.T) {
	src := &switchSource{err: fmt.Errorf("%w: %w", domain.ErrCatalogUnavailable, ErrBudgetExhausted)}
	c := NewBreakerCatalog(src, 3, time.Minute)

	for i := 0; i < 5; i++ {
		_, _ = c.Prices(context.Background(), []int{1})
	}
	assert.EqualValues(t, 5, src.calls.Load(), "нехватка бюджета — не вина соседа, предохранитель замкнут")
}

// ---------- ретраи ----------

type flakyServer struct {
	productv1.UnimplementedProductServiceServer
	calls    atomic.Int64
	failWith codes.Code
	failures int64
}

func (s *flakyServer) GetProducts(_ context.Context, req *productv1.GetProductsRequest) (*productv1.GetProductsResponse, error) {
	if n := s.calls.Add(1); n <= s.failures {
		return nil, status.Error(s.failWith, "flaky")
	}
	m, _ := MoneyToProto(decimal.NewFromInt(100), "RUB")
	return &productv1.GetProductsResponse{Products: []*productv1.Product{{Id: req.Ids[0], Price: m}}}, nil
}

func newRetryingClient(t *testing.T, s *flakyServer) *GRPCCatalog {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	productv1.RegisterProductServiceServer(srv, s)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(ServiceConfig))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return NewGRPCCatalog(conn, time.Second)
}

func TestRetry_UnavailableIsRetried(t *testing.T) {
	s := &flakyServer{failWith: codes.Unavailable, failures: 2}
	c := newRetryingClient(t, s)

	prices, err := c.Prices(context.Background(), []int{7})

	require.NoError(t, err, "сосед моргнул дважды — третья попытка прошла")
	assert.Contains(t, prices, 7)
	assert.EqualValues(t, 3, s.calls.Load())
}

func TestRetry_InvalidArgumentNotRetried(t *testing.T) {
	s := &flakyServer{failWith: codes.InvalidArgument, failures: 10}
	c := newRetryingClient(t, s)

	_, err := c.Prices(context.Background(), []int{7})

	assert.Error(t, err)
	assert.EqualValues(t, 1, s.calls.Load(), "ответ от повтора не изменится — не повторяем")
}
