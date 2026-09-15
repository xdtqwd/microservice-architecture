package retry

import (
	"context"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
	"order-service/internal/metrics"
)

const (
	maxAttempts = 5
	baseDelay   = 10 * time.Millisecond
	maxDelay    = 500 * time.Millisecond
)

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	pgErr, ok := err.(*pgconn.PgError)
	if !ok {
		return false
	}
	return pgErr.Code == "40001" || pgErr.Code == "40P01"
}

func Do(ctx context.Context, logger *zap.Logger, retries *int, fn func(ctx context.Context) error) error {
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := fn(ctx)
		if err == nil {
			return nil
		}
		if !isRetryable(err) {
			return err
		}
		*retries++
		metrics.RetryTotal.Inc()
		delay := baseDelay * (1 << attempt)
		if delay > maxDelay {
			delay = maxDelay
		}
		jitter := time.Duration(rand.Int63n(int64(delay)))
		logger.Warn("retrying after serialization conflict",
			zap.Int("attempt", attempt+1),
			zap.Error(err),
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay + jitter):
		}
	}
	return fn(ctx)
}
