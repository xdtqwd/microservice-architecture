package repository

import (
	"context"
	"errors"
	"order-service/internal/metrics"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type ctxKey string

const queryStartKey ctxKey = "query_start"

type queryTracer struct {
	logger         *zap.Logger
	acquireTimeout time.Duration
}

type acquireStartKey struct{}
type acquireCancelKey struct{}

// TraceAcquireStart ограничивает только ожидание соединения из пула.
// Контекст, который мы здесь возвращаем, пул использует для захвата,
// а сам запрос идёт уже с исходным контекстом.
func (t *queryTracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	ctx = context.WithValue(ctx, acquireStartKey{}, time.Now())
	if t.acquireTimeout <= 0 {
		return ctx
	}
	ctx, cancel := context.WithTimeout(ctx, t.acquireTimeout)
	return context.WithValue(ctx, acquireCancelKey{}, cancel)
}

func (t *queryTracer) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, data pgxpool.TraceAcquireEndData) {
	if start, ok := ctx.Value(acquireStartKey{}).(time.Time); ok {
		metrics.DBAcquireWait.Observe(time.Since(start).Seconds())
	}
	if cancel, ok := ctx.Value(acquireCancelKey{}).(context.CancelFunc); ok {
		cancel()
	}
	if errors.Is(data.Err, context.DeadlineExceeded) {
		metrics.DBAcquireTimeouts.Inc()
		t.logger.Warn("db pool exhausted: gave up waiting for connection",
			zap.Duration("waited", t.acquireTimeout))
	}
}

// PoolConfig — настройки пула. Откуда берутся значения — см. README.
type PoolConfig struct {
	MaxConns       int32
	MinConns       int32
	AcquireTimeout time.Duration
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	t.logger.Info("sql query", zap.String("sql", data.SQL))
	return context.WithValue(ctx, queryStartKey, time.Now())
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	start, _ := ctx.Value(queryStartKey).(time.Time)
	t.logger.Info("sql query done",
		zap.Duration("duration", time.Since(start)),
		zap.String("err", func() string {
			if data.Err != nil {
				return data.Err.Error()
			}
			return ""
		}()),
	)
}

func Connect(ctx context.Context, url string, logger *zap.Logger, pc PoolConfig) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.Tracer = &queryTracer{logger: logger, acquireTimeout: pc.AcquireTimeout}
	cfg.MaxConns = pc.MaxConns
	cfg.MinConns = pc.MinConns
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	return pgxpool.NewWithConfig(ctx, cfg)
}
