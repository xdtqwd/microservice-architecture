package txm

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ctxKey struct{}

type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type TxManager struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool}
}

func (m *TxManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if err := fn(context.WithValue(ctx, ctxKey{}, tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func Extract(ctx context.Context) pgx.Tx {
	tx, _ := ctx.Value(ctxKey{}).(pgx.Tx)
	return tx
}
