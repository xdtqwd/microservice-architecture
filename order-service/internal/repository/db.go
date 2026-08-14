package repository

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5"
)

func Connect() (*pgx.Conn, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://postgres:password@localhost:5436/orders_db"
	}
	return pgx.Connect(context.Background(), url)
}
