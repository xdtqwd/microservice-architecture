package main

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/lib/pq"
)

func connectDB() *sql.DB {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:password@postgres:5432/orders_db?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("db connect:", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatal("db ping:", err)
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS inbox (
			event_id     TEXT PRIMARY KEY,
			processed_at TIMESTAMPTZ DEFAULT NOW()
		)`)
	if err != nil {
		log.Fatal("create inbox:", err)
	}
	return db
}
