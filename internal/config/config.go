package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port             string
	DatabaseURL      string
	RedisURL         string
	RabbitMQURL      string
	WorkerCount      int
	QueueBuffer      int
	LockTTL          time.Duration
	JobTimeout       time.Duration
	ReaperInterval   time.Duration
	ReaperStaleAfter time.Duration
	APIKey           string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:             getEnv("PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		RedisURL:         os.Getenv("REDIS_URL"),
		RabbitMQURL:      os.Getenv("RABBITMQ_URL"),
		WorkerCount:      getEnvInt("WORKER_COUNT", 16),
		QueueBuffer:      getEnvInt("QUEUE_BUFFER", 1024),
		LockTTL:          getEnvDuration("LOCK_TTL", 10*time.Second),
		JobTimeout:       getEnvDuration("JOB_TIMEOUT", 5*time.Second),
		ReaperInterval:   getEnvDuration("REAPER_INTERVAL", 30*time.Second),
		ReaperStaleAfter: getEnvDuration("REAPER_STALE_AFTER", 60*time.Second),
		APIKey:           getEnv("API_KEY", "dev-secret-key-12345"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is required")
	}
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("REDIS_URL environment variable is required")
	}
	if cfg.RabbitMQURL == "" {
		return nil, fmt.Errorf("RABBITMQ_URL environment variable is required")
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
