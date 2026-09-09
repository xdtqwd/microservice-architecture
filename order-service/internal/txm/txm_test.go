package txm_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/jackc/pgx/v5/pgxpool"
	mytxm "order-service/internal/txm"
)

func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:password@localhost:5436/orders_db"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skip("no database available:", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Skip("no database available:", err)
	}
	return pool
}

func TestTxManager_Commit(t *testing.T) {
	pool := setupPool(t)
	defer pool.Close()
	tm := mytxm.New(pool)
	ctx := context.Background()

	err := tm.Do(ctx, func(ctx context.Context) error {
		tx := mytxm.Extract(ctx)
		assert.NotNil(t, tx)
		_, err := tx.Exec(ctx, "SELECT 1")
		return err
	})
	assert.NoError(t, err)
}

func TestTxManager_Rollback(t *testing.T) {
	pool := setupPool(t)
	defer pool.Close()
	tm := mytxm.New(pool)
	ctx := context.Background()

	_, err := pool.Exec(ctx, "CREATE TABLE IF NOT EXISTS txm_test (id SERIAL PRIMARY KEY, val TEXT)")
	assert.NoError(t, err)
	defer func() { _, _ = pool.Exec(ctx, "DROP TABLE txm_test") }()

	txErr := errors.New("rollback me")
	err = tm.Do(ctx, func(ctx context.Context) error {
		tx := mytxm.Extract(ctx)
		_, err := tx.Exec(ctx, "INSERT INTO txm_test (val) VALUES ('should not exist')")
		assert.NoError(t, err)
		return txErr
	})
	assert.ErrorIs(t, err, txErr)

	var count int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM txm_test").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 0, count, "транзакция должна была откатиться")
}
