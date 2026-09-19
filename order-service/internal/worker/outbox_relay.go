package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	kafkago "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

const maxRetries = 5
const batchSize = 100

var OutboxLagSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "outbox_lag_seconds",
	Help: "Age of the oldest unpublished outbox event in seconds",
})

func init() {
	prometheus.MustRegister(OutboxLagSeconds)
}

type OutboxRelay struct {
	pool     *pgxpool.Pool
	writer   *kafkago.Writer
	logger   *zap.Logger
	interval time.Duration
}

func NewOutboxRelay(pool *pgxpool.Pool, brokers []string, logger *zap.Logger) *OutboxRelay {
	return &OutboxRelay{
		pool: pool,
		writer: &kafkago.Writer{
			Addr:     kafkago.TCP(brokers...),
			Topic:    "orders",
			Balancer: &kafkago.Hash{},
		},
		logger:   logger,
		interval: 500 * time.Millisecond,
	}
}

func (r *OutboxRelay) Run(ctx context.Context) {
	r.logger.Info("outbox relay started")
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	defer func() { _ = r.writer.Close() }()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("outbox relay stopped")
			return
		case <-ticker.C:
			r.updateLagMetric(ctx)
			if err := r.processBatch(ctx); err != nil {
				r.logger.Error("outbox relay error", zap.Error(err))
			}
		}
	}
}

func (r *OutboxRelay) updateLagMetric(ctx context.Context) {
	var lagSeconds *float64
	err := r.pool.QueryRow(ctx,
		"SELECT EXTRACT(EPOCH FROM (NOW() - MIN(created_at))) FROM outbox WHERE published_at IS NULL").
		Scan(&lagSeconds)
	if err == nil && lagSeconds != nil {
		OutboxLagSeconds.Set(*lagSeconds)
	} else {
		OutboxLagSeconds.Set(0)
	}
}

func (r *OutboxRelay) processBatch(ctx context.Context) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT id, aggregate_id, event_type, payload
		FROM outbox
		WHERE published_at IS NULL AND retry_count < %d
		ORDER BY id
		LIMIT %d
		FOR UPDATE SKIP LOCKED`, maxRetries, batchSize))
	if err != nil {
		return err
	}
	defer rows.Close()

	var msgs []kafkago.Message
	var ids []int64

	for rows.Next() {
		var id int64
		var aggregateID int
		var eventType string
		var payload json.RawMessage

		if err := rows.Scan(&id, &aggregateID, &eventType, &payload); err != nil {
			return err
		}

		msgs = append(msgs, kafkago.Message{
			Key:   []byte(fmt.Sprintf("%d", aggregateID)),
			Value: payload,
		})
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(msgs) == 0 {
		return tx.Rollback(context.Background())
	}

	if err := r.writer.WriteMessages(ctx, msgs...); err != nil {
		// увеличиваем счётчик попыток
		_, _ = tx.Exec(ctx,
			"UPDATE outbox SET retry_count = retry_count + 1, failed_at = NOW() WHERE id = ANY($1)", ids)
		_ = tx.Commit(context.Background())

		// перемещаем в DLQ если превысили лимит
		r.moveToDLQ(ctx, ids, err.Error())
		return err
	}

	_, err = tx.Exec(ctx,
		"UPDATE outbox SET published_at = NOW() WHERE id = ANY($1)", ids)
	if err != nil {
		return err
	}

	return tx.Commit(context.Background())
}

func (r *OutboxRelay) moveToDLQ(ctx context.Context, ids []int64, errMsg string) {
	_, err := r.pool.Exec(context.Background(), `
		INSERT INTO outbox_dlq (aggregate_id, event_type, payload, error)
		SELECT aggregate_id, event_type, payload, $1
		FROM outbox WHERE id = ANY($2) AND retry_count >= $3`,
		errMsg, ids, maxRetries)
	if err != nil {
		r.logger.Error("failed to move to DLQ", zap.Error(err))
	}
}
