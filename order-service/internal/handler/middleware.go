package handler

import (
	"context"
	"net/http"
	"runtime/debug"
	"strconv"

	"order-service/internal/metrics"
	"order-service/internal/reqid"
	"time"

	"go.uber.org/zap"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := reqid.OrNew(r.Header.Get(reqid.Header))
		ctx := reqid.With(r.Context(), id)
		w.Header().Set(reqid.Header, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func Logger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			id := reqid.From(r.Context())
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
					id := reqid.From(r.Context())
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
