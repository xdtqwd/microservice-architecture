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

var (
	// Лейблы — только имя метода и gRPC-код: оба множества ограничены
	// (методы из .proto, 17 кодов). Никаких id и текстов ошибок — см. OPS-13.
	RPCClientRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rpc_client_requests_total",
		Help: "Outgoing RPC calls by method and code",
	}, []string{"method", "code"})
	RPCClientDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "rpc_client_duration_seconds",
		Help:    "Outgoing RPC call duration",
		Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
	}, []string{"method"})
)

func init() { prometheus.MustRegister(RPCClientRequests, RPCClientDuration) }
