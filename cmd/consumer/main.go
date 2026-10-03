package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rabbitmq/amqp091-go"

	"github.com/lucaasnogueira/buriti-pay/internal/config"
	"github.com/lucaasnogueira/buriti-pay/internal/messaging"
)

type NotificationService struct {
	logger *slog.Logger
}

func (s *NotificationService) Handle(ctx context.Context, routingKey string, body []byte) error {
	s.logger.Info("Received event in notification service",
		"routing_key", routingKey,
		"payload", string(body),
	)
	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("Starting Buriti Pay Consumer...")

	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Connect to RabbitMQ with retries
	var conn *amqp091.Connection
	for attempt := 1; attempt <= 10; attempt++ {
		conn, err = amqp091.Dial(cfg.RabbitMQURL)
		if err == nil {
			break
		}
		logger.Warn("Waiting for RabbitMQ broker...", "attempt", attempt, "error", err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		logger.Error("Failed to connect to RabbitMQ broker", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	// 2. Setup Topology
	tempCh, err := conn.Channel()
	if err != nil {
		logger.Error("Failed to open channel for topology setup", "error", err)
		os.Exit(1)
	}
	if err := messaging.SetupTopology(tempCh, logger); err != nil {
		logger.Error("Failed to declare topology", "error", err)
		os.Exit(1)
	}
	tempCh.Close()

	// 3. Initialize Consumer
	handler := &NotificationService{logger: logger}
	consumer := messaging.NewConsumer(conn, logger, handler, 20)
	if err := consumer.Start(ctx); err != nil {
		logger.Error("Failed to start consumer", "error", err)
		os.Exit(1)
	}

	// 4. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Info("Received shutdown signal", "signal", sig.String())

	consumer.Stop()
	fmt.Println("Buriti Pay Consumer stopped cleanly")
}
