package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
	_ "github.com/lib/pq"
)

type OrderEvent struct {
	OrderID int `json:"order_id"`
}

func StartConsumer(ctx context.Context, db *sql.DB) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{"kafka:9092"},
		Topic:   "orders",
		GroupID: "product-service",
		MaxWait: 1 * time.Second,
	})
	defer r.Close()

	log.Println("Kafka consumer started")

	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("consumer stopped")
				return
			}
			log.Printf("fetch error: %v", err)
			continue
		}

		var event OrderEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			log.Printf("unmarshal error: %v", err)
			_ = r.CommitMessages(ctx, msg)
			continue
		}

		eventID := fmt.Sprintf("order_created:%d", event.OrderID)

		processed, err := processWithInbox(ctx, db, eventID, func() error {
			log.Printf("processing order_created event: order_id=%d", event.OrderID)
			return nil
		})
		if err != nil {
			log.Printf("inbox error: %v", err)
			continue
		}
		if !processed {
			log.Printf("duplicate event %s, skipping", eventID)
		}

		if err := r.CommitMessages(ctx, msg); err != nil {
			log.Printf("commit error: %v", err)
		}
	}
}

// processWithInbox выполняет fn и записывает event_id в inbox в одной транзакции.
// Возвращает false если событие уже было обработано (ON CONFLICT DO NOTHING).
func processWithInbox(ctx context.Context, db *sql.DB, eventID string, fn func() error) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	// INSERT ... ON CONFLICT DO NOTHING
	// rows affected = 0 означает дубль
	res, err := tx.ExecContext(ctx,
		"INSERT INTO inbox (event_id, processed_at) VALUES ($1, NOW()) ON CONFLICT DO NOTHING",
		eventID)
	if err != nil {
		return false, err
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		// дубль — коммитим транзакцию без побочных эффектов
		return false, tx.Commit()
	}

	// выполняем основную логику обработки
	if err := fn(); err != nil {
		return false, err
	}

	return true, tx.Commit()
}
