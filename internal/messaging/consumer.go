package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/rabbitmq/amqp091-go"
)

type EventHandler interface {
	Handle(ctx context.Context, routingKey string, body []byte) error
}

type Consumer struct {
	conn       *amqp091.Connection
	logger     *slog.Logger
	handler    EventHandler
	prefetch   int
	processed  map[string]bool
	mu         sync.Mutex
	stopChan   chan struct{}
}

func NewConsumer(conn *amqp091.Connection, logger *slog.Logger, handler EventHandler, prefetch int) *Consumer {
	if prefetch <= 0 {
		prefetch = 10
	}
	return &Consumer{
		conn:      conn,
		logger:    logger,
		handler:   handler,
		prefetch:  prefetch,
		processed: make(map[string]bool),
		stopChan:  make(chan struct{}),
	}
}

// Start begins consuming from notifications.q with manual acks and client deduplication.
func (c *Consumer) Start(ctx context.Context) error {
	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open consumer channel: %w", err)
	}

	if err := ch.Qos(c.prefetch, 0, false); err != nil {
		ch.Close()
		return fmt.Errorf("failed to set prefetch qos: %w", err)
	}

	deliveries, err := ch.Consume(
		NotificationsQueue,
		"buriti-notifications-consumer",
		false, // manual ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		ch.Close()
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	c.logger.Info("Consumer subscribed to notifications queue", "prefetch", c.prefetch)

	go func() {
		defer ch.Close()

		for {
			select {
			case <-c.stopChan:
				c.logger.Info("Consumer stopped")
				return
			case <-ctx.Done():
				c.logger.Info("Consumer context cancelled, stopping")
				return
			case msg, ok := <-deliveries:
				if !ok {
					c.logger.Warn("Consumer channel closed")
					return
				}
				c.processMessage(ctx, msg)
			}
		}
	}()

	return nil
}

func (c *Consumer) Stop() {
	close(c.stopChan)
}

func (c *Consumer) processMessage(ctx context.Context, msg amqp091.Delivery) {
	// 1. Deduplication check by message/event ID
	var raw map[string]any
	dedupKey := msg.MessageId
	if err := json.Unmarshal(msg.Body, &raw); err == nil {
		if eid, ok := raw["event_id"].(string); ok && eid != "" {
			dedupKey = eid
		} else if pid, ok := raw["payment_id"].(string); ok && pid != "" {
			dedupKey = pid
		}
	}

	if dedupKey != "" {
		c.mu.Lock()
		if c.processed[dedupKey] {
			c.mu.Unlock()
			c.logger.Info("Duplicate event discarded by consumer", "dedup_key", dedupKey)
			_ = msg.Ack(false)
			return
		}
		c.processed[dedupKey] = true
		c.mu.Unlock()
	}

	// 2. Process message via handler
	if c.handler != nil {
		if err := c.handler.Handle(ctx, msg.RoutingKey, msg.Body); err != nil {
			c.logger.Error("Event processing failed, nacking message to trigger DLX/retry",
				"routing_key", msg.RoutingKey,
				"error", err,
			)
			// Nack without requeue (routes message to DLX/DLQ)
			_ = msg.Nack(false, false)
			return
		}
	}

	// 3. Positively acknowledge message
	_ = msg.Ack(false)
}
