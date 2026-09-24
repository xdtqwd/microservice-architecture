package repository_test

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		fmt.Println("skipping repository integration tests (-short)")
		os.Exit(0)
	}
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	pg, err := postgres.Run(ctx, "postgres:15-alpine",
		postgres.WithDatabase("orders_test"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Println("start postgres:", err)
		return 1
	}
	defer func() { _ = pg.Terminate(ctx) }()

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Println("dsn:", err)
		return 1
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Println("open:", err)
		return 1
	}
	if err := goose.SetDialect("postgres"); err != nil {
		fmt.Println("goose dialect:", err)
		return 1
	}
	if err := goose.Up(db, "../../migrations/goose"); err != nil {
		fmt.Println("migrate:", err)
		return 1
	}
	_ = db.Close()

	testPool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Println("pool:", err)
		return 1
	}
	defer testPool.Close()

	return m.Run()
}

func resetDB(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`TRUNCATE orders, order_items, products, idempotency_keys, outbox, outbox_dlq
		 RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
}

func seedProduct(t *testing.T, name string, price, stock int) int {
	t.Helper()
	var id int
	err := testPool.QueryRow(context.Background(),
		"INSERT INTO products (name, price, stock) VALUES ($1, $2, $3) RETURNING id",
		name, price, stock).Scan(&id)
	require.NoError(t, err)
	return id
}

func stockOf(t *testing.T, id int) int {
	t.Helper()
	var s int
	require.NoError(t, testPool.QueryRow(context.Background(),
		"SELECT stock FROM products WHERE id = $1", id).Scan(&s))
	return s
}
