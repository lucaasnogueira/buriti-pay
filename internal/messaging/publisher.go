package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

type RabbitPublisher struct {
	conn   *amqp091.Connection
	ch     *amqp091.Channel
	logger *slog.Logger
	mu     sync.Mutex
}

func NewRabbitPublisher(conn *amqp091.Connection, logger *slog.Logger) (*RabbitPublisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open rabbitmq channel: %w", err)
	}

	// Enable Publisher Confirms
	if err := ch.Confirm(false); err != nil {
		ch.Close()
		return nil, fmt.Errorf("failed to enable publisher confirms: %w", err)
	}

	// Ensure topology
	if err := SetupTopology(ch, logger); err != nil {
		ch.Close()
		return nil, err
	}

	return &RabbitPublisher{
		conn:   conn,
		ch:     ch,
		logger: logger,
	}, nil
}

func (p *RabbitPublisher) Publish(ctx context.Context, routingKey string, body []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	confirmation, err := p.ch.PublishWithDeferredConfirmWithContext(
		ctx,
		EventsExchange,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp091.Publishing{
			DeliveryMode: amqp091.Persistent,
			ContentType:  "application/json",
			Body:         body,
			Timestamp:    time.Now().UTC(),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	// Wait for publisher confirm
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-confirmation.Done():
		if !confirmation.Acked() {
			return fmt.Errorf("rabbitmq nack received for published message")
		}
	}

	return nil
}

func (p *RabbitPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ch != nil {
		return p.ch.Close()
	}
	return nil
}
