package lock

import (
	"context"
	"errors"
	"time"
)

var (
	ErrLockNotAcquired = errors.New("failed to acquire distributed lock")
	ErrLockNotHeld     = errors.New("lock is not held or owned by caller")
)

type LockHandle struct {
	Key       string
	Token     string
	stopWatch chan struct{}
}

// Locker defines the distributed locking contract.
type Locker interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (*LockHandle, error)
	Release(ctx context.Context, handle *LockHandle) error
	Extend(ctx context.Context, handle *LockHandle, ttl time.Duration) error
}
