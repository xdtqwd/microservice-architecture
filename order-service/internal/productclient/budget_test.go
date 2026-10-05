package productclient

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"order-service/internal/domain"
	"order-service/internal/gen/productv1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// deadlineServer запоминает, какой дедлайн доехал до сервера, и ждёт отмены.
type deadlineServer struct {
	productv1.UnimplementedProductServiceServer
	// пишутся в горутине сервера, читаются в тесте — только атомарно
	gotBudget atomic.Int64
	called    atomic.Bool
	sawCancel atomic.Bool
}

func (s *deadlineServer) GetProducts(ctx context.Context, _ *productv1.GetProductsRequest) (*productv1.GetProductsResponse, error) {
	s.called.Store(true)
	if dl, ok := ctx.Deadline(); ok {
		s.gotBudget.Store(int64(time.Until(dl)))
	}
	select {
	case <-ctx.Done(): // сервер честно бросает работу, когда клиент ушёл
		s.sawCancel.Store(true)
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		return &productv1.GetProductsResponse{}, nil
	}
}

func newDeadlineClient(t *testing.T, s *deadlineServer, timeout time.Duration) *GRPCCatalog {
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

// У запроса 600мс, резерв 300мс: соседу достаётся ~300мс, а не потолок в 1с.
func TestBudget_ShrinksToWhatIsLeft(t *testing.T) {
	s := &deadlineServer{}
	c := newDeadlineClient(t, s, time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := c.Prices(ctx, []int{1})

	assert.ErrorIs(t, err, domain.ErrCatalogUnavailable)
	assert.LessOrEqual(t, time.Duration(s.gotBudget.Load()), 300*time.Millisecond, "дедлайн доехал до сервера уже урезанным")
	assert.Greater(t, time.Duration(s.gotBudget.Load()), 200*time.Millisecond)
	assert.Less(t, time.Since(start), 450*time.Millisecond, "резерв на свою работу остался")
	assert.Eventually(t, func() bool { return s.sawCancel.Load() }, time.Second, 10*time.Millisecond,
		"сервер увидел отмену и бросил работу")
}

// Осталось 200мс при резерве 300мс — соседа не трогаем вовсе.
func TestBudget_ExhaustedSkipsCall(t *testing.T) {
	s := &deadlineServer{}
	c := newDeadlineClient(t, s, time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := c.Prices(ctx, []int{1})

	assert.ErrorIs(t, err, domain.ErrCatalogUnavailable)
	assert.False(t, s.called.Load(), "заведомо не успеем — не звоним")
	assert.Less(t, time.Since(start), 10*time.Millisecond, "отказ мгновенный")
}

// Без дедлайна у запроса — обычный потолок вызова.
func TestBudget_NoParentDeadline_UsesTimeout(t *testing.T) {
	s := &deadlineServer{}
	c := newDeadlineClient(t, s, 100*time.Millisecond)

	_, err := c.Prices(context.Background(), []int{1})

	assert.ErrorIs(t, err, domain.ErrCatalogUnavailable)
	assert.LessOrEqual(t, time.Duration(s.gotBudget.Load()), 100*time.Millisecond)
}
