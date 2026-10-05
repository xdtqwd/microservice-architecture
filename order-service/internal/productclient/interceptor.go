package productclient

import (
	"context"
	"path"

	"order-service/internal/metrics"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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
