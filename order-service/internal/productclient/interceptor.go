package productclient

import (
	"context"
	"path"
	"time"

	"order-service/internal/metrics"
	"order-service/internal/reqid"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// DeadlineMetrics считает исходящие вызовы, оборванные дедлайном или отменой.
func DeadlineMetrics(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	err := invoker(ctx, method, req, reply, cc, opts...)
	if code := status.Code(err); code == codes.DeadlineExceeded || code == codes.Canceled {
		metrics.RPCClientDeadline.WithLabelValues(path.Base(method), code.String()).Inc()
	}
	return err
}

// RequestIDPropagation кладёт request_id из контекста в метаданные вызова —
// так он переезжает в соседний сервис.
func RequestIDPropagation(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if id := reqid.From(ctx); id != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, reqid.MetadataKey, id)
	}
	return invoker(ctx, method, req, reply, cc, opts...)
}

// ClientObserver — лог и метрики исходящего вызова, по образцу HTTP-middleware.
func ClientObserver(logger *zap.Logger) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		dur := time.Since(start)
		code := status.Code(err)
		name := method // полное имя, как на сервере: удобно сопоставлять

		metrics.RPCClientRequests.WithLabelValues(name, code.String()).Inc()
		metrics.RPCClientDuration.WithLabelValues(name).Observe(dur.Seconds())
		logger.Info("grpc call",
			zap.String("request_id", reqid.From(ctx)),
			zap.String("method", method),
			zap.String("code", code.String()),
			zap.Duration("duration", dur))
		return err
	}
}
