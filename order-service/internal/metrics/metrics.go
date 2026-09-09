package metrics

import "github.com/prometheus/client_golang/prometheus"

var RetryTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "order_retry_total",
	Help: "Total number of transaction retries due to serialization conflicts",
})

func init() {
	prometheus.MustRegister(RetryTotal)
}
