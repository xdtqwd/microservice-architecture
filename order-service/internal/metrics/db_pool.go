package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// Время ожидания свободного соединения. Занятость пула через Stat()
	// раз в 5 секунд этого не видит: соединение занято миллисекунды.
	DBAcquireWait = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "db_pool_acquire_wait_seconds",
		Help:    "Time spent waiting for a connection from the pool",
		Buckets: []float64{.0005, .001, .005, .01, .05, .1, .25, .5, 1, 2},
	})
	DBAcquireTimeouts = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "db_pool_acquire_timeouts_total",
		Help: "Requests that gave up waiting for a pool connection",
	})
)

func init() {
	prometheus.MustRegister(DBAcquireWait, DBAcquireTimeouts)
}
