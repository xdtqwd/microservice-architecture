package repository_test

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"order-service/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"go.uber.org/zap"
)

var (
	testPool      *pgxpool.Pool
	testDSN       string
	testRedisAddr string
	testRedis     *goredis.Client
)

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

	pg, err := tcpostgres.Run(ctx, "postgres:15-alpine",
		tcpostgres.WithDatabase("orders_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
		// быстрее ловить дедлоки в тестах (по умолчанию 1s)
		testcontainers.WithCmdArgs("-c", "deadlock_timeout=100ms"),
	)
	if err != nil {
		fmt.Println("start postgres:", err)
		return 1
	}
	defer func() { _ = pg.Terminate(ctx) }()

	rd, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		fmt.Println("start redis:", err)
		return 1
	}
	defer func() { _ = rd.Terminate(ctx) }()

	testRedisAddr, err = rd.Endpoint(ctx, "")
	if err != nil {
		fmt.Println("redis endpoint:", err)
		return 1
	}
	testRedis = goredis.NewClient(&goredis.Options{Addr: testRedisAddr})
	defer func() { _ = testRedis.Close() }()

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	testDSN = dsn
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

	// через Connect, а не pgxpool.New — заодно проверяем настройки пула и трейсер
	testPool, err = repository.Connect(ctx, dsn, zap.NewNop(), repository.PoolConfig{MaxConns: 5, MinConns: 1, AcquireTimeout: time.Second})
	if err != nil {
		fmt.Println("pool:", err)
		return 1
	}
	defer testPool.Close()

	return m.Run()
}

// resetDB очищает Postgres и Redis. Тесты в пакете идут последовательно.
func resetDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := testPool.Exec(ctx,
		`TRUNCATE orders, order_items, products, idempotency_keys, outbox, outbox_dlq
		 RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
	require.NoError(t, testRedis.FlushAll(ctx).Err())
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

func countRows(t *testing.T, table string) int {
	t.Helper()
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM "+table).Scan(&n))
	return n
}

func setStatus(t *testing.T, orderID int, status string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		"UPDATE orders SET status = $1 WHERE id = $2", status, orderID)
	require.NoError(t, err)
}
