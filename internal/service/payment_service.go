package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/lucaasnogueira/buriti-pay/internal/domain"
	"github.com/lucaasnogueira/buriti-pay/internal/lock"
	"github.com/lucaasnogueira/buriti-pay/internal/repository"
	"github.com/lucaasnogueira/buriti-pay/internal/worker"
)

type PaymentService interface {
	CreatePaymentAsync(ctx context.Context, idempotencyKey string, fromID, toID uuid.UUID, amount int64) (*domain.Payment, error)
	GetPayment(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	Process(ctx context.Context, job worker.Job) error
}

type DefaultPaymentService struct {
	repo     repository.PaymentRepository
	locker   lock.Locker
	enqueuer worker.Enqueuer
	lockTTL  time.Duration
	logger   *slog.Logger
}

func NewPaymentService(
	repo repository.PaymentRepository,
	locker lock.Locker,
	enqueuer worker.Enqueuer,
	lockTTL time.Duration,
	logger *slog.Logger,
) *DefaultPaymentService {
	if lockTTL <= 0 {
		lockTTL = 10 * time.Second
	}
	return &DefaultPaymentService{
		repo:     repo,
		locker:   locker,
		enqueuer: enqueuer,
		lockTTL:  lockTTL,
		logger:   logger,
	}
}

// CreatePaymentAsync handles the non-blocking ingestion of a payment (Phase 3, ADR-0001).
func (s *DefaultPaymentService) CreatePaymentAsync(
	ctx context.Context,
	idempotencyKey string,
	fromID, toID uuid.UUID,
	amount int64,
) (*domain.Payment, error) {
	newPayment, err := domain.NewPayment(idempotencyKey, fromID, toID, amount)
	if err != nil {
		return nil, err
	}

	err = s.repo.Create(ctx, newPayment)
	if err != nil {
		if errors.Is(err, repository.ErrUniqueViolation) {
			existing, getErr := s.repo.GetByIdempotencyKey(ctx, idempotencyKey)
			if getErr != nil {
				return nil, fmt.Errorf("failed to retrieve conflicting payment: %w", getErr)
			}

			if !existing.MatchesPayload(fromID, toID, amount) {
				return nil, domain.ErrIdempotencyConflict
			}

			return existing, nil
		}
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	// Enqueue to the in-memory bounded channel
	if s.enqueuer != nil {
		if err := s.enqueuer.Enqueue(worker.Job{PaymentID: newPayment.ID}); err != nil {
			return nil, err
		}
	}

	return newPayment, nil
}

// Process implements worker.JobHandler with distributed locking and deterministic lock ordering.
// ADR-0004:
// 1. Locks accounts in ascending UUID order to prevent deadlocks across concurrent transfers.
// 2. Acquires Redis locks with TTL and renewal.
// 3. Fallbacks gracefully to database-level row locking if Redis is unavailable or unconfigured.
// 4. Safely releases locks in reverse order after completion.
func (s *DefaultPaymentService) Process(ctx context.Context, job worker.Job) error {
	payment, err := s.repo.GetByID(ctx, job.PaymentID)
	if err != nil {
		return fmt.Errorf("failed to load payment for job: %w", err)
	}

	if payment.Status == domain.StatusConfirmed || payment.Status == domain.StatusFailed {
		// Terminal state already reached
		return nil
	}

	// Deterministic order: ascending UUID
	firstID, secondID := payment.FromAccountID, payment.ToAccountID
	if firstID.String() > secondID.String() {
		firstID, secondID = secondID, firstID
	}

	var handles []*lock.LockHandle
	defer func() {
		// Release locks in reverse order
		for i := len(handles) - 1; i >= 0; i-- {
			if s.locker != nil && handles[i] != nil {
				_ = s.locker.Release(ctx, handles[i])
			}
		}
	}()

	// Acquire Redis locks if locker is configured
	if s.locker != nil {
		key1 := fmt.Sprintf("lock:account:%s", firstID.String())
		h1, err := s.locker.Acquire(ctx, key1, s.lockTTL)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("Redis lock acquisition failed, falling back to database locking", "key", key1, "error", err)
			}
		} else {
			handles = append(handles, h1)
		}

		key2 := fmt.Sprintf("lock:account:%s", secondID.String())
		h2, err := s.locker.Acquire(ctx, key2, s.lockTTL)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("Redis lock acquisition failed, falling back to database locking", "key", key2, "error", err)
			}
		} else {
			handles = append(handles, h2)
		}
	}

	// Execute atomic transfer in PostgreSQL (Fencing + Ledger + Outbox)
	_, err = s.repo.ExecuteTransfer(ctx, job.PaymentID)
	return err
}

func (s *DefaultPaymentService) GetPayment(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	return s.repo.GetByID(ctx, id)
}
