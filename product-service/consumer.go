package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

type OrderEvent struct {
	OrderID int `json:"order_id"`
}

type InboxStore struct {
	processed map[string]bool
}

func NewInboxStore() *InboxStore {
	return &InboxStore{processed: make(map[string]bool)}
}

func (s *InboxStore) IsProcessed(eventID string) bool {
	return s.processed[eventID]
}

func (s *InboxStore) MarkProcessed(eventID string) {
	s.processed[eventID] = true
}

func StartConsumer(ctx context.Context, inbox *InboxStore) {
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
			r.CommitMessages(ctx, msg)
			continue
		}

		// ключ дедупликации — order_id + partition + offset
		// order_id уникален для каждого заказа
		// но при at-least-once одно сообщение может прийти дважды с разным offset
		// поэтому используем только order_id как семантический ключ
		eventID := fmt.Sprintf("order_created:%d", event.OrderID)

		if inbox.IsProcessed(eventID) {
			log.Printf("duplicate event %s, skipping", eventID)
			r.CommitMessages(ctx, msg)
			continue
		}

		log.Printf("processing order_created event: order_id=%d", event.OrderID)
		inbox.MarkProcessed(eventID)

		if err := r.CommitMessages(ctx, msg); err != nil {
			log.Printf("commit error: %v", err)
		}
	}
}
