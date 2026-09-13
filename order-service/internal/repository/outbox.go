package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"order-service/internal/txm"
)

type OutboxEvent struct {
	ID          int64
	AggregateID int
	EventType   string
	Payload     json.RawMessage
	CreatedAt   time.Time
	PublishedAt *time.Time
}

type OutboxRepo struct {
	pool *pgxpool.Pool
}

func NewOutboxRepo(pool *pgxpool.Pool) *OutboxRepo {
	return &OutboxRepo{pool: pool}
}

func (r *OutboxRepo) Insert(ctx context.Context, aggregateID int, eventType string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	q := r.querier(ctx)
	_, err = q.Exec(ctx,
		"INSERT INTO outbox (aggregate_id, event_type, payload) VALUES ($1, $2, $3)",
		aggregateID, eventType, data)
	return err
}

func (r *OutboxRepo) querier(ctx context.Context) Querier {
	if tx := txm.Extract(ctx); tx != nil {
		return tx
	}
	return r.pool
}
