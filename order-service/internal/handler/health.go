package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	db    Pinger
	cache Pinger
}

func NewHealthHandler(db Pinger, cache Pinger) *HealthHandler {
	return &HealthHandler{db: db, cache: cache}
}

func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	// Проверки параллельно и каждая со своим таймаутом: раньше они шли
	// по очереди с общим дедлайном, и медленная база съедала всё время,
	// после чего живой Redis тоже отчитывался как недоступный.
	type result struct {
		name string
		err  error
	}
	checks := map[string]Pinger{"db": h.db, "cache": h.cache}
	results := make(chan result, len(checks))
	for name, p := range checks {
		go func(name string, p Pinger) {
			ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
			defer cancel()
			results <- result{name, p.Ping(ctx)}
		}(name, p)
	}

	errs := map[string]string{}
	for range checks {
		if res := <-results; res.err != nil {
			errs[res.name] = res.err.Error()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if len(errs) > 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "unavailable", "errors": errs})
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

const readinessTimeout = 1500 * time.Millisecond
