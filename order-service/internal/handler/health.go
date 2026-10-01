package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	db           Pinger
	cache        Pinger
	shuttingDown atomic.Bool
}

func NewHealthHandler(db Pinger, cache Pinger) *HealthHandler {
	return &HealthHandler{db: db, cache: cache}
}

// SetShuttingDown переводит /readyz в 503 до закрытия порта.
func (h *HealthHandler) SetShuttingDown() { h.shuttingDown.Store(true) }

// readinessTimeout — проверка лёгкая (Ping), но с пределом:
// readiness не должна сама становиться нагрузкой или висеть.
const readinessTimeout = 1500 * time.Millisecond

// Liveness — процесс жив и обрабатывает запросы. Никаких внешних зависимостей:
// при лежащей базе перезапуск сервиса ничего не лечит, только добавляет рестартов.
func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readiness — готов ли принимать трафик прямо сейчас.
// Решает только то, без чего работать нельзя: база.
// Redis проверяется справочно: после FAIL-01 сервис без него работает,
// и выкидывать инстанс из балансировки из-за кеша нельзя.
func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	if h.shuttingDown.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "shutting_down"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	cacheErr := make(chan error, 1)
	go func() { cacheErr <- h.cache.Ping(ctx) }()
	dbErr := h.db.Ping(ctx)

	checks := map[string]string{"db": "ok", "cache": "ok"}
	if err := <-cacheErr; err != nil {
		checks["cache"] = "degraded: " + err.Error()
	}

	if dbErr != nil {
		checks["db"] = dbErr.Error()
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "unavailable", "checks": checks})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "checks": checks})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
