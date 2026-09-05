package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type ctxKey string

const queryStartKey ctxKey = "query_start"

type queryTracer struct {
	logger *zap.Logger
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	t.logger.Debug("sql query", zap.String("sql", data.SQL))
	return context.WithValue(ctx, queryStartKey, time.Now())
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	start, _ := ctx.Value(queryStartKey).(time.Time)
	t.logger.Debug("sql query done",
		zap.Duration("duration", time.Since(start)),
		zap.String("err", func() string {
			if data.Err != nil {
				return data.Err.Error()
			}
			return ""
		}()),
	)
}

func Connect(ctx context.Context, url string, logger *zap.Logger) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	config.ConnConfig.Tracer = &queryTracer{logger: logger}
	return pgxpool.NewWithConfig(ctx, config)
}
