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
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	errs := map[string]string{}

	if err := h.db.Ping(ctx); err != nil {
		errs["db"] = err.Error()
	}
	if err := h.cache.Ping(ctx); err != nil {
		errs["cache"] = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	if len(errs) > 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"status": "unavailable", "errors": errs})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
