package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"order-service/internal/domain"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

type IdempotencyStore interface {
	Claim(ctx context.Context, key, requestHash string) (domain.IdempotencyClaim, error)
	Complete(ctx context.Context, key string, code int, body []byte) error
	Release(ctx context.Context, key string) error
}

// idempotencyLeaderTimeout — предел для исполнения запроса лидером.
// Меньше дедлайна запроса (10s) и аренды ключа (15s).
const idempotencyLeaderTimeout = 8 * time.Second

type storedResponse struct {
	code       int
	body       []byte
	retryAfter bool
}

// Idempotent требует Idempotency-Key и гарантирует: повтор с тем же ключом и
// тем же запросом получает ответ первой попытки, не выполняя её заново.
func Idempotent(store IdempotencyStore, logger *zap.Logger, next http.Handler) http.Handler {
	var group singleflight.Group
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" || len(key) > 255 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Idempotency-Key header is required for payments"})
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<16))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "cannot read request body"})
			return
		}
		sum := sha256.Sum256([]byte(r.Method + " " + r.URL.Path + "\n" + string(body)))
		hash := hex.EncodeToString(sum[:])

		// ключ singleflight включает отпечаток: запрос с тем же ключом, но другим
		// телом не должен получить чужой ответ — он пойдёт отдельно и упрётся в 422
		ch := group.DoChan(key+":"+hash, func() (interface{}, error) {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), idempotencyLeaderTimeout)
			defer cancel()
			return execIdempotent(ctx, store, logger, next, r, key, hash, body), nil
		})

		select {
		case <-r.Context().Done():
			return
		case res := <-ch:
			resp := res.Val.(storedResponse)
			w.Header().Set("Content-Type", "application/json")
			if resp.retryAfter {
				w.Header().Set("Retry-After", "1")
			}
			w.WriteHeader(resp.code)
			_, _ = w.Write(resp.body)
		}
	})
}

func execIdempotent(ctx context.Context, store IdempotencyStore, logger *zap.Logger,
	next http.Handler, r *http.Request, key, hash string, body []byte) storedResponse {

	claim, err := store.Claim(ctx, key, hash)
	if err != nil {
		logger.Error("idempotency claim failed", zap.String("key", key), zap.Error(err))
		return jsonResponse(http.StatusServiceUnavailable, "Service Unavailable", true)
	}
	switch {
	case claim.Mismatch:
		return jsonResponse(http.StatusUnprocessableEntity,
			"Idempotency-Key was already used with a different request", false)
	case claim.Busy:
		return jsonResponse(http.StatusConflict,
			"request with this Idempotency-Key is still in progress, retry later", true)
	case !claim.Owner:
		return storedResponse{code: claim.Code, body: claim.Body}
	}

	req := r.Clone(ctx)
	req.Body = io.NopCloser(bytes.NewReader(body))
	rec := &captureWriter{header: http.Header{}, code: http.StatusOK}
	next.ServeHTTP(rec, req)

	if rec.code >= 500 {
		// исход неизвестен или временный сбой — ответ не запоминаем,
		// повтор выполнится заново; провайдер по тому же ключу не спишет дважды
		if err := store.Release(ctx, key); err != nil {
			logger.Error("idempotency release failed", zap.String("key", key), zap.Error(err))
		}
		return storedResponse{code: rec.code, body: rec.buf.Bytes(), retryAfter: rec.code == http.StatusServiceUnavailable}
	}
	if err := store.Complete(ctx, key, rec.code, rec.buf.Bytes()); err != nil {
		logger.Error("idempotency complete failed", zap.String("key", key), zap.Error(err))
	}
	return storedResponse{code: rec.code, body: rec.buf.Bytes()}
}

func jsonResponse(code int, msg string, retryAfter bool) storedResponse {
	b, _ := json.Marshal(errorResponse{Error: msg})
	return storedResponse{code: code, body: append(b, '\n'), retryAfter: retryAfter}
}

type captureWriter struct {
	header http.Header
	code   int
	buf    bytes.Buffer
}

func (c *captureWriter) Header() http.Header         { return c.header }
func (c *captureWriter) Write(b []byte) (int, error) { return c.buf.Write(b) }
func (c *captureWriter) WriteHeader(code int)        { c.code = code }
