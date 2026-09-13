package worker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	kafkago "github.com/segmentio/kafka-go"
	"fmt"
	"go.uber.org/zap"
)

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
			if err := r.processBatch(ctx); err != nil {
				r.logger.Error("outbox relay error", zap.Error(err))
			}
		}
	}
}

func (r *OutboxRelay) processBatch(ctx context.Context) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	rows, err := tx.Query(ctx, `
		SELECT id, aggregate_id, event_type, payload
		FROM outbox
		WHERE published_at IS NULL
		ORDER BY id
		LIMIT 100
		FOR UPDATE SKIP LOCKED`)
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

		key := []byte(fmt.Sprintf("%d", aggregateID))
		msgs = append(msgs, kafkago.Message{
			Key:   key,
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
		return err
	}

	_, err = tx.Exec(ctx,
		"UPDATE outbox SET published_at = NOW() WHERE id = ANY($1)", ids)
	if err != nil {
		return err
	}

	return tx.Commit(context.Background())
}
