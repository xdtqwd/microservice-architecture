package handler

import (
	"context"
	"net/http"
	"runtime/debug"
	"strconv"

	"order-service/internal/metrics"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type contextKey string

const requestIDKey contextKey = "request_id"

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.New().String()
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func Logger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			id, _ := r.Context().Value(requestIDKey).(string)
			dur := time.Since(start)
			logger.Info("request",
				zap.String("request_id", id),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", rw.status),
				zap.Duration("duration", dur),
			)
			metrics.HTTPDuration.WithLabelValues(
				r.Method,
				r.URL.Path,
				strconv.Itoa(rw.status),
			).Observe(dur.Seconds())
		})
	}
}

func Recover(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					id, _ := r.Context().Value(requestIDKey).(string)
					logger.Error("panic recovered",
						zap.String("request_id", id),
						zap.Any("error", err),
						zap.ByteString("stack", debug.Stack()),
					)
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

// RequestTimeout — общий дедлайн запроса.
const RequestTimeout = 10 * time.Second

// Chain — цепочка middleware, которой обёрнут весь сервер.
// Вынесена сюда, чтобы тесты проверяли ровно ту цепочку, что работает в проде.
func Chain(logger *zap.Logger, next http.Handler) http.Handler {
	return RequestID(
		Logger(logger)(
			Recover(logger)(
				Timeout(RequestTimeout)(next),
			),
		),
	)
}
