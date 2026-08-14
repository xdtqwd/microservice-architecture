package repository

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect() (*pgxpool.Pool, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://postgres:password@localhost:5436/orders_db"
	}
	return pgxpool.New(context.Background(), url)
}
