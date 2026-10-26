package main

import (
	"context"
	"net"
	"strings"
	"testing"

	"product-service/gen/productv1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// Сервер с той же цепочкой интерцепторов, что в проде. Пул базы пустой:
// любой запрос с id паникует — проверяем recover без переключателей.
func newTestClient(t *testing.T) productv1.ProductServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(requestID, accessLog, recoverPanic, observe))
	productv1.RegisterProductServiceServer(srv, &catalogServer{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return productv1.NewProductServiceClient(conn)
}

func TestRecover_PanicBecomesInternal_ServerAlive(t *testing.T) {
	c := newTestClient(t)

	_, err := c.GetProducts(context.Background(), &productv1.GetProductsRequest{Ids: []int64{1}})
	if status.Code(err) != codes.Internal {
		t.Fatalf("паника должна стать Internal, получили %v", err)
	}

	// процесс жив: следующий вызов обслуживается
	if _, err := c.GetProducts(context.Background(), &productv1.GetProductsRequest{}); err != nil {
		t.Fatalf("после паники сервер должен отвечать: %v", err)
	}
}

func callID(t *testing.T, c productv1.ProductServiceClient, sent string) string {
	t.Helper()
	ctx := context.Background()
	if sent != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, requestIDKey, sent)
	}
	var hdr metadata.MD
	if _, err := c.GetProducts(ctx, &productv1.GetProductsRequest{}, grpc.Header(&hdr)); err != nil {
		t.Fatal(err)
	}
	got := hdr.Get(requestIDKey)
	if len(got) == 0 {
		t.Fatal("сервер не вернул request_id")
	}
	return got[0]
}

func TestRequestID_PropagatedFromClient(t *testing.T) {
	if got := callID(t, newTestClient(t), "req-123"); got != "req-123" {
		t.Fatalf("ожидали req-123, получили %q", got)
	}
}

func TestRequestID_GeneratedWhenMissing(t *testing.T) {
	if got := callID(t, newTestClient(t), ""); got == "" {
		t.Fatal("без id от клиента сервер должен создать свой")
	}
}

// Перевод строки отбивает уже сам HTTP/2: клиент не отправит такие метаданные.
// Проверяем то, что протокол пропускает, а мы — нет: пробелы и кавычки
// (подделка полей в текстовом логе) и слишком длинный id.
func TestRequestID_UnsafeReplaced(t *testing.T) {
	c := newTestClient(t)
	for _, bad := range []string{
		`x" level="ERROR" msg="fake`,
		strings.Repeat("a", 65),
	} {
		if got := callID(t, c, bad); got == bad {
			t.Fatalf("небезопасный id %q нельзя пускать в логи как есть", bad)
		}
	}
}
