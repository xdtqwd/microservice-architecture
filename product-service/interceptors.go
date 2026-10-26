package main

import (
	"context"
	"log/slog"
	"os"
	"regexp"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const requestIDKey = "x-request-id"

var (
	logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

	safeID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

	// Лейблы: полное имя метода и gRPC-код — оба множества ограничены.
	rpcServerRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rpc_server_requests_total",
		Help: "Handled RPC calls by method and code",
	}, []string{"method", "code"})
	rpcServerDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "rpc_server_duration_seconds",
		Help:    "RPC handling duration",
		Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
	}, []string{"method"})
)

func init() { prometheus.MustRegister(rpcServerRequests, rpcServerDuration) }

type reqIDKey struct{}

func reqID(ctx context.Context) string {
	id, _ := ctx.Value(reqIDKey{}).(string)
	return id
}

// requestID достаёт id из метаданных вызова или создаёт новый,
// кладёт в контекст и возвращает клиенту в заголовке ответа.
func requestID(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	id := ""
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get(requestIDKey); len(v) > 0 && safeID.MatchString(v[0]) {
			id = v[0]
		}
	}
	if id == "" {
		id = uuid.New().String()
	}
	_ = grpc.SetHeader(ctx, metadata.Pairs(requestIDKey, id))
	return handler(context.WithValue(ctx, reqIDKey{}, id), req)
}

// accessLog — лог и метрики вызова: метод, код, длительность, request_id.
func accessLog(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	dur := time.Since(start)
	code := status.Code(err)

	rpcServerRequests.WithLabelValues(info.FullMethod, code.String()).Inc()
	rpcServerDuration.WithLabelValues(info.FullMethod).Observe(dur.Seconds())
	logger.Info("grpc request",
		"request_id", reqID(ctx),
		"method", info.FullMethod,
		"code", code.String(),
		"duration_ms", float64(dur.Microseconds())/1000)
	return resp, err
}

// recoverPanic: паника в обработчике не роняет процесс, клиент получает Internal.
func recoverPanic(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	defer func() {
		if p := recover(); p != nil {
			logger.Error("grpc panic",
				"request_id", reqID(ctx),
				"method", info.FullMethod,
				"panic", p,
				"stack", string(debug.Stack()))
			resp, err = nil, status.Error(codes.Internal, "internal error")
		}
	}()
	return handler(ctx, req)
}
