package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lucaasnogueira/buriti-pay/internal/domain"
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
	enqueuer worker.Enqueuer
}

func NewPaymentService(repo repository.PaymentRepository, enqueuer worker.Enqueuer) *DefaultPaymentService {
	return &DefaultPaymentService{
		repo:     repo,
		enqueuer: enqueuer,
	}
}

// CreatePaymentAsync handles the non-blocking ingestion of a payment (Phase 3, ADR-0001).
// Steps:
// 1. Idempotency check: returns existing if already present and matches payload.
// 2. Persists payment as PENDING in PostgreSQL.
// 3. Enqueues job to bounded channel. If full, returns worker.ErrQueueFull (triggers HTTP 429).
// 4. Returns payment with status PENDING for immediate HTTP 202 response.
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

// Process implements worker.JobHandler, executing atomic fund transfer in the worker pool.
func (s *DefaultPaymentService) Process(ctx context.Context, job worker.Job) error {
	_, err := s.repo.ExecuteTransfer(ctx, job.PaymentID)
	return err
}

func (s *DefaultPaymentService) GetPayment(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	return s.repo.GetByID(ctx, id)
}
