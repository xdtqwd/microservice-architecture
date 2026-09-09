package kafka

import (
	"context"
	"encoding/json"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafkago.Writer
}

func NewProducer(brokers []string) *Producer {
	return &Producer{
		writer: &kafkago.Writer{
			Addr:     kafkago.TCP(brokers...),
			Topic:    "orders",
			Balancer: &kafkago.LeastBytes{},
		},
	}
}

func (p *Producer) SendOrderCreated(ctx context.Context, orderID int) error {
	payload, err := json.Marshal(map[string]interface{}{
		"order_id":   orderID,
		"event":      "order_created",
		"created_at": time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafkago.Message{
		Value: payload,
	})
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
