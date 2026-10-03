package http

import (
	"context"
	"net/http"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

type HealthChecker interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	dbChecker    HealthChecker
	redisClient  *redis.Client
	rabbitConnFn func() (*amqp091.Connection, error)
}

func NewHealthHandler(dbChecker HealthChecker, redisClient *redis.Client, rabbitConnFn func() (*amqp091.Connection, error)) *HealthHandler {
	return &HealthHandler{
		dbChecker:    dbChecker,
		redisClient:  redisClient,
		rabbitConnFn: rabbitConnFn,
	}
}

func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := make(map[string]string)
	allHealthy := true

	// Check Postgres
	if h.dbChecker != nil {
		if err := h.dbChecker.Ping(ctx); err != nil {
			checks["postgres"] = "unhealthy: " + err.Error()
			allHealthy = false
		} else {
			checks["postgres"] = "ok"
		}
	}

	// Check Redis
	if h.redisClient != nil {
		if err := h.redisClient.Ping(ctx).Err(); err != nil {
			checks["redis"] = "unhealthy: " + err.Error()
			allHealthy = false
		} else {
			checks["redis"] = "ok"
		}
	}

	// Check RabbitMQ
	if h.rabbitConnFn != nil {
		conn, err := h.rabbitConnFn()
		if err != nil || conn.IsClosed() {
			checks["rabbitmq"] = "unhealthy"
			allHealthy = false
		} else {
			checks["rabbitmq"] = "ok"
		}
	}

	status := http.StatusOK
	if !allHealthy {
		status = http.StatusServiceUnavailable
	}

	respondJSON(w, status, map[string]any{
		"ready":  allHealthy,
		"checks": checks,
	})
}
