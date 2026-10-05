package main

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	"product-service/gen/productv1"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// Сколько id можно спросить одним вызовом: защита от запроса на весь каталог.
const maxBatch = 500

type catalogServer struct {
	productv1.UnimplementedProductServiceServer
	pool *pgxpool.Pool
}

func (s *catalogServer) GetProduct(ctx context.Context, req *productv1.GetProductRequest) (*productv1.GetProductResponse, error) {
	if req.GetId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "id must be positive")
	}
	ps, err := s.load(ctx, []int64{req.GetId()})
	if err != nil {
		return nil, err
	}
	if len(ps) == 0 {
		return nil, status.Errorf(codes.NotFound, "product %d not found", req.GetId())
	}
	return &productv1.GetProductResponse{Product: ps[0]}, nil
}

func (s *catalogServer) GetProducts(ctx context.Context, req *productv1.GetProductsRequest) (*productv1.GetProductsResponse, error) {
	ids := req.GetIds()
	if len(ids) > maxBatch {
		return nil, status.Errorf(codes.InvalidArgument, "at most %d ids per call", maxBatch)
	}
	resp := &productv1.GetProductsResponse{}
	if len(ids) == 0 {
		return resp, nil
	}
	ps, err := s.load(ctx, ids)
	if err != nil {
		return nil, err
	}
	found := make(map[int64]bool, len(ps))
	for _, p := range ps {
		found[p.Id] = true
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if !found[id] && !seen[id] {
			resp.NotFoundIds = append(resp.NotFoundIds, id)
		}
		seen[id] = true
	}
	resp.Products = ps
	return resp, nil
}

// Демо-переключатели RPC-03: медленная база и сервер, глухой к отмене.
var (
	slowQuery, _ = time.ParseDuration(os.Getenv("CATALOG_SLOW_QUERY"))
	ignoreCtx    = os.Getenv("CATALOG_IGNORE_CTX") == "1"
)

// load — один запрос на всю пачку, а не по товару за раз.
// Запросы в базу идут с контекстом вызова: клиент ушёл — pgx отменяет
// запрос в Postgres, и база не считает то, что никто не ждёт.
func (s *catalogServer) load(ctx context.Context, ids []int64) ([]*productv1.Product, error) {
	qctx := ctx
	if ignoreCtx {
		qctx = context.Background() // демо: так делать нельзя
	}
	if slowQuery > 0 {
		if _, err := s.pool.Exec(qctx, "SELECT pg_sleep($1)", slowQuery.Seconds()); err != nil {
			return nil, ctxOrUnavailable(ctx, err)
		}
	}

	rows, err := s.pool.Query(qctx, "SELECT id, name, price, stock FROM products WHERE id = ANY($1)", ids)
	if err != nil {
		return nil, ctxOrUnavailable(ctx, err)
	}
	defer rows.Close()

	var out []*productv1.Product
	for rows.Next() {
		var p productv1.Product
		var price decimal.Decimal
		if err := rows.Scan(&p.Id, &p.Name, &price, &p.Stock); err != nil {
			log.Printf("catalog scan: %v", err)
			return nil, status.Error(codes.Internal, "catalog read failed")
		}
		if p.Price, err = moneyToProto(price, "RUB"); err != nil {
			log.Printf("catalog price %d: %v", p.Id, err)
			return nil, status.Error(codes.Internal, "invalid price in catalog")
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Error(codes.Unavailable, "catalog unavailable")
	}
	return out, nil
}

// startGRPC поднимает gRPC-сервер каталога на GRPC_ADDR (по умолчанию :9090).
func startGRPC() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required for the catalog gRPC server")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("catalog db: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("catalog db ping: %v", err)
	}

	addr := os.Getenv("GRPC_ADDR")
	if addr == "" {
		addr = ":9090"
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("grpc listen: %v", err)
	}
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		requestID,    // первым: id нужен всем, включая лог паники
		accessLog,    // видит итоговый код, в том числе Internal после паники
		recoverPanic, // паника -> Internal, процесс жив
		observe,      // RPC-03: ушёл ли клиент раньше, чем сервер закончил
	))
	productv1.RegisterProductServiceServer(srv, &catalogServer{pool: pool})
	reflection.Register(srv) // чтобы можно было смотреть сервис через grpcurl

	go func() {
		log.Printf("catalog gRPC server on %s", addr)
		if err := srv.Serve(lis); err != nil {
			log.Fatalf("grpc serve: %v", err)
		}
	}()
}

// ctxOrUnavailable: если клиент ушёл, честно отвечаем отменой или дедлайном,
// а не «каталог недоступен» — иначе по метрикам не отличить сбой базы от
// того, что запрос просто никто не ждал.
func ctxOrUnavailable(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return status.FromContextError(ctx.Err()).Err()
	}
	log.Printf("catalog query: %v", err)
	return status.Error(codes.Unavailable, "catalog unavailable")
}
