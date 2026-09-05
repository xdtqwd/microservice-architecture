package metrics

import "github.com/prometheus/client_golang/prometheus"

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
	prometheus.MustRegister(CacheHits, CacheMisses)
}
