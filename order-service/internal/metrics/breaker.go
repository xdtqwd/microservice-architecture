package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// 0 — closed, 1 — open, 2 — half_open
	BreakerState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "circuit_breaker_state",
		Help: "Circuit breaker state: 0 closed, 1 open, 2 half_open",
	}, []string{"name"})
	BreakerTransitions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "circuit_breaker_transitions_total",
		Help: "Circuit breaker state transitions",
	}, []string{"name", "from", "to"})
)

func init() {
	prometheus.MustRegister(BreakerState, BreakerTransitions)
}
