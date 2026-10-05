package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// Вызовы соседей, оборванные по дедлайну или отмене на нашей стороне.
	RPCClientDeadline = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rpc_client_deadline_total",
		Help: "Outgoing RPC calls ended by deadline or cancellation",
	}, []string{"method", "code"})
	// Вызовы, которые не делали: бюджета запроса уже не хватало.
	RPCClientSkipped = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rpc_client_skipped_total",
		Help: "Outgoing RPC calls not made because the request budget was exhausted",
	}, []string{"method"})
)

func init() { prometheus.MustRegister(RPCClientDeadline, RPCClientSkipped) }
