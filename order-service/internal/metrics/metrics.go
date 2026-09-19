package metrics

import "github.com/prometheus/client_golang/prometheus"

var RetryTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "order_retry_total",
	Help: "Total number of transaction retries due to serialization conflicts",
})

var HTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "http_request_duration_seconds",
	Help:    "HTTP request duration in seconds",
	Buckets: prometheus.DefBuckets,
}, []string{"method", "path", "status"})

var DBPoolAcquired = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "db_pool_acquired_conns",
	Help: "Number of currently acquired connections in pgxpool",
})

var DBPoolIdle = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "db_pool_idle_conns",
	Help: "Number of idle connections in pgxpool",
})

var (
	CacheHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total cache hits by level",
		},
		[]string{"level"},
	)
	CacheMisses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total cache misses by level",
		},
		[]string{"level"},
	)
)

func init() {
	prometheus.MustRegister(
		RetryTotal,
		HTTPDuration,
		DBPoolAcquired,
		DBPoolIdle,
		CacheHits,
		CacheMisses,
	)
}
