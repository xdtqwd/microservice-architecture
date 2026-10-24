package main

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var rpcServerDeadline = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "rpc_server_client_gone_total",
	Help: "Calls where the client left before the server finished: abandoned — server stopped, wasted — server finished for nobody",
}, []string{"method", "outcome"})

func init() { prometheus.MustRegister(rpcServerDeadline) }

// observe логирует каждый вызов: сколько оставалось у клиента и что стало с работой.
func observe(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	budget := "none"
	if dl, ok := ctx.Deadline(); ok {
		budget = time.Until(dl).Round(time.Millisecond).String()
	}

	resp, err := handler(ctx, req)

	took := time.Since(start).Round(time.Millisecond)
	if ctx.Err() == nil {
		return resp, err
	}
	code := status.Code(err)
	outcome := "wasted"
	if code == codes.DeadlineExceeded || code == codes.Canceled {
		outcome = "abandoned"
	}
	rpcServerDeadline.WithLabelValues(info.FullMethod, outcome).Inc()
	logger.Warn("client gone",
		"request_id", reqID(ctx),
		"method", info.FullMethod,
		"reason", ctx.Err().Error(),
		"client_budget", budget,
		"server_worked", took.String(),
		"outcome", outcome)
	return resp, err
}
