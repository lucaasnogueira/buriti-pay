package messaging

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/rabbitmq/amqp091-go"
)

const (
	EventsExchange      = "payments.events"
	NotificationsQueue  = "notifications.q"
	NotificationsDLQ    = "notifications.dlq"
	DLXExchange         = "payments.dlx"
	PaymentConfirmedKey = "payment.confirmed"
	PaymentFailedKey    = "payment.failed"
)

// SetupTopology declares exchanges, queues, DLX/DLQ, and bindings.
func SetupTopology(ch *amqp091.Channel, logger *slog.Logger) error {
	// 1. Declare Main Topic Exchange
	err := ch.ExchangeDeclare(
		EventsExchange,
		"topic",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to declare exchange %s: %w", EventsExchange, err)
	}

	// 2. Declare DLX Exchange
	err = ch.ExchangeDeclare(
		DLXExchange,
		"direct",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to declare dlx %s: %w", DLXExchange, err)
	}

	// 3. Declare DLQ
	_, err = ch.QueueDeclare(
		NotificationsDLQ,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to declare dlq %s: %w", NotificationsDLQ, err)
	}

	// Bind DLQ to DLX
	err = ch.QueueBind(
		NotificationsDLQ,
		NotificationsDLQ,
		DLXExchange,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind dlq to dlx: %w", err)
	}

	// 4. Declare Main Queue with DLX configured
	mainArgs := amqp091.Table{
		"x-dead-letter-exchange":    DLXExchange,
		"x-dead-letter-routing-key": NotificationsDLQ,
	}
	_, err = ch.QueueDeclare(
		NotificationsQueue,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		mainArgs,
	)
	if err != nil {
		return fmt.Errorf("failed to declare main queue %s: %w", NotificationsQueue, err)
	}

	// Bind Main Queue to Topic Exchange for payment.*
	err = ch.QueueBind(
		NotificationsQueue,
		"payment.*",
		EventsExchange,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind main queue: %w", err)
	}

	logger.Info("RabbitMQ topology initialized successfully",
		"exchange", EventsExchange,
		"queue", NotificationsQueue,
		"dlq", NotificationsDLQ,
	)
	return nil
}

type EventPublisher interface {
	Publish(ctx context.Context, routingKey string, body []byte) error
}
