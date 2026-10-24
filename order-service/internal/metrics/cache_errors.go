package metrics

import "github.com/prometheus/client_golang/prometheus"

// CacheErrors — ошибки кеша, после которых мы деградируем в следующий уровень.
// Без этой метрики упавший Redis незаметен: ответы 200, только медленнее.
var CacheErrors = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "cache_errors_total",
		Help: "Cache backend errors that were degraded to the next level",
	},
	[]string{"level", "op"},
)

func init() {
	prometheus.MustRegister(CacheErrors)
}
