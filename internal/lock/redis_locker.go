package lock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// Lua script to atomically release lock only if token matches
	unlockLuaScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
else
    return 0
end
`

	// Lua script to atomically extend TTL only if token matches
	extendLuaScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("PEXPIRE", KEYS[1], ARGV[2])
else
    return 0
end
`
)

type RedisLocker struct {
	client     *redis.Client
	logger     *slog.Logger
	unlockScript *redis.Script
	extendScript *redis.Script
	maxRetries int
	baseRetry  time.Duration
}

func NewRedisLocker(client *redis.Client, logger *slog.Logger) *RedisLocker {
	return &RedisLocker{
		client:       client,
		logger:       logger,
		unlockScript: redis.NewScript(unlockLuaScript),
		extendScript: redis.NewScript(extendLuaScript),
		maxRetries:   5,
		baseRetry:    30 * time.Millisecond,
	}
}

func (l *RedisLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (*LockHandle, error) {
	token, err := randomToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate lock token: %w", err)
	}

	for attempt := 0; attempt <= l.maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// SET lock:key token NX PX ttl
		ok, err := l.client.SetNX(ctx, key, token, ttl).Result()
		if err == nil && ok {
			handle := &LockHandle{
				Key:       key,
				Token:     token,
				stopWatch: make(chan struct{}),
			}
			l.startWatchdog(handle, ttl)
			return handle, nil
		}

		if attempt < l.maxRetries {
			backoff := l.calculateBackoff(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	return nil, ErrLockNotAcquired
}

func (l *RedisLocker) Release(ctx context.Context, handle *LockHandle) error {
	if handle == nil {
		return nil
	}

	// Stop renewal watchdog
	if handle.stopWatch != nil {
		select {
		case <-handle.stopWatch:
		default:
			close(handle.stopWatch)
		}
	}

	res, err := l.unlockScript.Run(ctx, l.client, []string{handle.Key}, handle.Token).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}
		return fmt.Errorf("redis unlock error: %w", err)
	}

	if val, ok := res.(int64); ok && val == 0 {
		return ErrLockNotHeld
	}

	return nil
}

func (l *RedisLocker) Extend(ctx context.Context, handle *LockHandle, ttl time.Duration) error {
	if handle == nil {
		return ErrLockNotHeld
	}

	res, err := l.extendScript.Run(ctx, l.client, []string{handle.Key}, handle.Token, ttl.Milliseconds()).Result()
	if err != nil {
		return fmt.Errorf("redis extend error: %w", err)
	}

	if val, ok := res.(int64); ok && val == 0 {
		return ErrLockNotHeld
	}

	return nil
}

func (l *RedisLocker) startWatchdog(handle *LockHandle, ttl time.Duration) {
	// Renew approximately at 1/3 of TTL interval
	interval := ttl / 3
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-handle.stopWatch:
				return
			case <-ticker.C:
				extendCtx, cancel := context.WithTimeout(context.Background(), interval/2)
				err := l.Extend(extendCtx, handle, ttl)
				cancel()
				if err != nil {
					l.logger.Warn("Lock watchdog renewal failed", "key", handle.Key, "error", err)
					return
				}
			}
		}
	}()
}

func (l *RedisLocker) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff with jitter
	multiplier := math.Pow(2, float64(attempt))
	base := float64(l.baseRetry) * multiplier

	jitterBig, _ := rand.Int(rand.Reader, big.NewInt(20))
	jitterMs := time.Duration(jitterBig.Int64()) * time.Millisecond

	return time.Duration(base) + jitterMs
}

func randomToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
