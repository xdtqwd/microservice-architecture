package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SchemaPinger — проверка готовности базы для /readyz: база отвечает
// и схема не старше той, что нужна этому образу. Ping сам по себе
// проходил и на пустой базе — и под получал трафик, на котором
// каждый заказ падал.
type SchemaPinger struct {
	pool *pgxpool.Pool
	want int64
}

func NewSchemaPinger(pool *pgxpool.Pool, want int64) *SchemaPinger {
	return &SchemaPinger{pool: pool, want: want}
}

func (p *SchemaPinger) Ping(ctx context.Context) error {
	var have int64
	err := p.pool.QueryRow(ctx,
		"SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied").Scan(&have)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" { // undefined_table
		return errors.New("schema not migrated: goose_db_version is missing")
	}
	if err != nil {
		return err
	}
	if have < p.want {
		return fmt.Errorf("schema version %d, service needs %d", have, p.want)
	}
	return nil
}
