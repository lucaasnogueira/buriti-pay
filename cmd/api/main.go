package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"

	"github.com/lucaasnogueira/buriti-pay/internal/config"
	apphttp "github.com/lucaasnogueira/buriti-pay/internal/http"
	"github.com/lucaasnogueira/buriti-pay/internal/repository"
	"github.com/lucaasnogueira/buriti-pay/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("Starting Buriti Pay API...")

	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. PostgreSQL Pool
	dbPool, err := repository.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("Failed to initialize PostgreSQL connection pool", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	accountRepo := repository.NewPostgresAccountRepository(dbPool)
	paymentRepo := repository.NewPostgresPaymentRepository(dbPool)

	// 2. Services
	paymentService := service.NewPaymentService(paymentRepo)

	// 3. Redis Client
	redisOpt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Error("Failed to parse Redis URL", "error", err)
		os.Exit(1)
	}
	redisClient := redis.NewClient(redisOpt)
	defer redisClient.Close()

	// 4. RabbitMQ Connection Provider
	rabbitConnFn := func() (*amqp091.Connection, error) {
		return amqp091.Dial(cfg.RabbitMQURL)
	}

	// 5. Handlers & Router
	healthHandler := apphttp.NewHealthHandler(accountRepo, redisClient, rabbitConnFn)
	accountHandler := apphttp.NewAccountHandler(accountRepo)
	paymentHandler := apphttp.NewPaymentHandler(paymentService)

	router := apphttp.NewRouter(logger, cfg.APIKey, healthHandler, accountHandler, paymentHandler)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 6. Graceful shutdown handler
	shutdownErrChan := make(chan error, 1)
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quit
		logger.Info("Received shutdown signal", "signal", sig.String())

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			shutdownErrChan <- fmt.Errorf("graceful shutdown failed: %w", err)
			return
		}
		shutdownErrChan <- nil
	}()

	logger.Info("Buriti Pay API server listening", "port", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("HTTP server failed to start", "error", err)
		os.Exit(1)
	}

	if err := <-shutdownErrChan; err != nil {
		logger.Error("Error during shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("Buriti Pay API server stopped successfully")
}
