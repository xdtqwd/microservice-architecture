package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	kafkago "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

const (
	maxRetries = 5
	batchSize  = 100

	// потолок экспоненциальной задержки, когда брокер недоступен
	maxBackoff = 30 * time.Second

	// опубликованные строки храним сутки: чтобы можно было разобрать
	// инцидент и переотправить события, если потеряет Kafka или консьюмер
	retention       = 24 * time.Hour
	cleanupInterval = time.Minute
	cleanupBatch    = 5000
)

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

// nextBackoff удваивает задержку от base до max.
func nextBackoff(cur, base, max time.Duration) time.Duration {
	next := cur * 2
	if next < base {
		next = base
	}
	if next > max {
		return max
	}
	return next
}

func (r *OutboxRelay) Run(ctx context.Context) {
	r.logger.Info("outbox relay started")
	defer func() { _ = r.writer.Close() }()

	timer := time.NewTimer(0)
	defer timer.Stop()
	delay := r.interval
	lastCleanup := time.Now()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("outbox relay stopped")
			return
		case <-timer.C:
		}

		r.updateLagMetric(ctx)

		n, err := r.processBatch(ctx)
		switch {
		case err != nil:
			// не долбим лежащую Kafka: 0.5s, 1s, 2s ... до 30s
			delay = nextBackoff(delay, r.interval, maxBackoff)
			r.logger.Warn("outbox publish failed, backing off",
				zap.Duration("next_try_in", delay), zap.Error(err))
		case n == batchSize:
			// пачка полная — очередь не пуста, разгребаем без паузы
			delay = 0
		default:
			delay = r.interval
		}

		if time.Since(lastCleanup) >= cleanupInterval {
			r.cleanup(ctx)
			lastCleanup = time.Now()
		}

		timer.Reset(delay)
	}
}

// updateLagMetric берёт самое старое неопубликованное событие через
// частичный индекс idx_outbox_unpublished: ORDER BY id LIMIT 1 вместо MIN(created_at),
// который сканировал всю таблицу.
func (r *OutboxRelay) updateLagMetric(ctx context.Context) {
	var createdAt *time.Time
	err := r.pool.QueryRow(ctx,
		"SELECT created_at FROM outbox WHERE published_at IS NULL ORDER BY id LIMIT 1").
		Scan(&createdAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		OutboxLagSeconds.Set(0)
	case err != nil:
		r.logger.Warn("outbox lag query failed", zap.Error(err)) // оставляем прошлое значение
	case createdAt != nil:
		OutboxLagSeconds.Set(time.Since(*createdAt).Seconds())
	}
}

// processBatch отправляет пачку и возвращает число опубликованных событий.
func (r *OutboxRelay) processBatch(ctx context.Context) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
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
		return 0, err
	}

	var msgs []kafkago.Message
	var ids []int64
	for rows.Next() {
		var id int64
		var aggregateID int
		var eventType string
		var payload json.RawMessage
		if err := rows.Scan(&id, &aggregateID, &eventType, &payload); err != nil {
			rows.Close()
			return 0, err
		}
		msgs = append(msgs, kafkago.Message{
			Key:   []byte(fmt.Sprintf("%d", aggregateID)),
			Value: payload,
		})
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(msgs) == 0 {
		return 0, nil
	}

	werr := r.writer.WriteMessages(ctx, msgs...)
	if werr == nil {
		if _, err := tx.Exec(ctx, "UPDATE outbox SET published_at = NOW() WHERE id = ANY($1)", ids); err != nil {
			return 0, err
		}
		return len(ids), tx.Commit(context.Background())
	}

	// kafka-go для пачки возвращает WriteErrors даже когда брокер лежит целиком —
	// у каждого сообщения тогда своя копия сетевой ошибки. Поэтому различаем
	// не форму ошибки, а её природу: попытку считаем только за постоянную.
	var perMsg kafkago.WriteErrors
	if !errors.As(werr, &perMsg) {
		if isPoison(werr) && len(ids) == 1 {
			perMsg = kafkago.WriteErrors{werr}
		} else {
			return 0, fmt.Errorf("kafka unavailable: %w", werr)
		}
	}

	var published, failed []int64
	for i, e := range perMsg {
		switch {
		case e == nil:
			published = append(published, ids[i])
		case isPoison(e):
			failed = append(failed, ids[i])
		default:
			// временная ошибка — строку не трогаем, попытку не считаем
		}
	}
	if len(published) == 0 && len(failed) == 0 {
		return 0, fmt.Errorf("kafka unavailable: %w", werr)
	}
	if len(published) > 0 {
		if _, err := tx.Exec(ctx, "UPDATE outbox SET published_at = NOW() WHERE id = ANY($1)", published); err != nil {
			return 0, err
		}
	}
	if len(failed) > 0 {
		if _, err := tx.Exec(ctx,
			"UPDATE outbox SET retry_count = retry_count + 1, failed_at = NOW() WHERE id = ANY($1)", failed); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(context.Background()); err != nil {
		return 0, err
	}
	if len(failed) > 0 {
		r.moveToDLQ(failed, werr.Error())
	}
	return len(published), werr
}

// isPoison — постоянная ошибка, которая не пройдёт от повтора:
// слишком большое или битое сообщение. Сетевые ошибки и временные ответы
// брокера (нет лидера партиции, таймаут) — не вина сообщения.
func isPoison(err error) bool {
	var kerr kafkago.Error
	if errors.As(err, &kerr) {
		return !kerr.Temporary()
	}
	return false
}

// moveToDLQ переносит события, исчерпавшие попытки: удаление из outbox и вставка
// в outbox_dlq одним запросом. Раньше событие только копировалось и навсегда
// оставалось в outbox с published_at IS NULL, держа метрику лага растущей.
func (r *OutboxRelay) moveToDLQ(ids []int64, errMsg string) {
	_, err := r.pool.Exec(context.Background(), `
		WITH moved AS (
			DELETE FROM outbox
			WHERE id = ANY($2) AND retry_count >= $3
			RETURNING aggregate_id, event_type, payload
		)
		INSERT INTO outbox_dlq (aggregate_id, event_type, payload, error)
		SELECT aggregate_id, event_type, payload, $1 FROM moved`,
		errMsg, ids, maxRetries)
	if err != nil {
		r.logger.Error("failed to move to DLQ", zap.Error(err))
	}
}

// cleanup удаляет опубликованные события старше retention небольшими пачками,
// чтобы не держать долгую блокировку и не раздувать WAL одним большим DELETE.
// Старые опубликованные строки лежат в начале по id, поэтому подзапрос находит их
// по первичному ключу без отдельного индекса по published_at.
func (r *OutboxRelay) cleanup(ctx context.Context) {
	cutoff := time.Now().Add(-retention)
	total := int64(0)
	for i := 0; i < 20; i++ {
		tag, err := r.pool.Exec(ctx, `
			DELETE FROM outbox WHERE id IN (
				SELECT id FROM outbox
				WHERE published_at IS NOT NULL AND published_at < $1
				ORDER BY id LIMIT $2
			)`, cutoff, cleanupBatch)
		if err != nil {
			r.logger.Warn("outbox cleanup failed", zap.Error(err))
			return
		}
		total += tag.RowsAffected()
		if tag.RowsAffected() < cleanupBatch {
			break
		}
	}
	if total > 0 {
		r.logger.Info("outbox cleanup", zap.Int64("deleted", total))
	}
}
