package productclient

import (
	"context"
	"errors"
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

type fakeServer struct {
	productv1.UnimplementedProductServiceServer
	calls atomic.Int64
	err   error
	delay time.Duration
	asked [][]int64
}

func (s *fakeServer) GetProducts(_ context.Context, req *productv1.GetProductsRequest) (*productv1.GetProductsResponse, error) {
	s.calls.Add(1)
	s.asked = append(s.asked, req.Ids)
	time.Sleep(s.delay)
	if s.err != nil {
		return nil, s.err
	}
	catalog := map[int64]string{1: "150000.50", 2: "99.99"}
	resp := &productv1.GetProductsResponse{}
	for _, id := range req.Ids {
		price, ok := catalog[id]
		if !ok {
			resp.NotFoundIds = append(resp.NotFoundIds, id)
			continue
		}
		m, _ := MoneyToProto(decimal.RequireFromString(price), "RUB")
		resp.Products = append(resp.Products, &productv1.Product{Id: id, Price: m})
	}
	return resp, nil
}

func newClient(t *testing.T, s *fakeServer, timeout time.Duration) *GRPCCatalog {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	productv1.RegisterProductServiceServer(srv, s)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return NewGRPCCatalog(conn, timeout)
}

func TestGRPCCatalog_OneCallExactPrices(t *testing.T) {
	s := &fakeServer{}
	c := newClient(t, s, time.Second)

	prices, err := c.Prices(context.Background(), []int{1, 2, 3, 1})

	require.NoError(t, err)
	assert.EqualValues(t, 1, s.calls.Load(), "весь заказ — один вызов, не N")
	assert.Equal(t, []int64{1, 2, 3}, s.asked[0], "повторы id не отправляем")
	assert.True(t, decimal.RequireFromString("150000.50").Equal(prices[1]))
	assert.True(t, decimal.RequireFromString("99.99").Equal(prices[2]))
	assert.NotContains(t, prices, 3, "отсутствующий товар — просто нет в ответе")
}

func TestGRPCCatalog_Unavailable(t *testing.T) {
	c := newClient(t, &fakeServer{err: status.Error(codes.Unavailable, "down")}, time.Second)
	_, err := c.Prices(context.Background(), []int{1})
	assert.ErrorIs(t, err, domain.ErrCatalogUnavailable)
}

func TestGRPCCatalog_SlowNeighbour_OwnTimeout(t *testing.T) {
	c := newClient(t, &fakeServer{delay: 300 * time.Millisecond}, 50*time.Millisecond)

	start := time.Now()
	_, err := c.Prices(context.Background(), []int{1})

	assert.ErrorIs(t, err, domain.ErrCatalogUnavailable)
	assert.Less(t, time.Since(start), 200*time.Millisecond, "не ждём медленного соседа дольше своего таймаута")
}

// источник для теста кеша
type countingSource struct {
	asked [][]int
	err   error
}

func (s *countingSource) Prices(_ context.Context, ids []int) (map[int]decimal.Decimal, error) {
	s.asked = append(s.asked, append([]int(nil), ids...))
	if s.err != nil {
		return nil, s.err
	}
	out := map[int]decimal.Decimal{}
	for _, id := range ids {
		if id != 404 {
			out[id] = decimal.NewFromInt(int64(id * 100))
		}
	}
	return out, nil
}

func TestCachedCatalog_AsksOnlyForMissing(t *testing.T) {
	src := &countingSource{}
	c := NewCachedCatalog(src, 100, time.Minute)

	_, err := c.Prices(context.Background(), []int{1, 2})
	require.NoError(t, err)
	prices, err := c.Prices(context.Background(), []int{1, 2, 3})
	require.NoError(t, err)

	assert.Equal(t, [][]int{{1, 2}, {3}}, src.asked, "второй раз спросили только про новый товар")
	assert.Len(t, prices, 3)
}

func TestCachedCatalog_AllCached_NoCall(t *testing.T) {
	src := &countingSource{}
	c := NewCachedCatalog(src, 100, time.Minute)
	_, _ = c.Prices(context.Background(), []int{1, 2})
	_, _ = c.Prices(context.Background(), []int{2, 1})
	assert.Len(t, src.asked, 1)
}

func TestCachedCatalog_TTLExpires(t *testing.T) {
	src := &countingSource{}
	c := NewCachedCatalog(src, 100, 30*time.Millisecond)
	_, _ = c.Prices(context.Background(), []int{1})
	time.Sleep(60 * time.Millisecond)
	_, _ = c.Prices(context.Background(), []int{1})
	assert.Len(t, src.asked, 2, "после TTL цену спрашиваем снова — изменение цены дойдёт")
}

func TestCachedCatalog_NotFoundAndErrorsNotCached(t *testing.T) {
	src := &countingSource{}
	c := NewCachedCatalog(src, 100, time.Minute)
	_, _ = c.Prices(context.Background(), []int{404})
	_, _ = c.Prices(context.Background(), []int{404})
	assert.Len(t, src.asked, 2, "отсутствие товара не кешируем")

	src.err = errors.New("down")
	_, err := c.Prices(context.Background(), []int{7})
	assert.Error(t, err)
	src.err = nil
	_, err = c.Prices(context.Background(), []int{7})
	assert.NoError(t, err, "ошибка не залипла в кеше")
}
