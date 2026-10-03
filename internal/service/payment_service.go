package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lucaasnogueira/buriti-pay/internal/domain"
	"github.com/lucaasnogueira/buriti-pay/internal/repository"
)

type PaymentService interface {
	ProcessPayment(ctx context.Context, idempotencyKey string, fromID, toID uuid.UUID, amount int64) (*domain.Payment, error)
	GetPayment(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
}

type DefaultPaymentService struct {
	repo repository.PaymentRepository
}

func NewPaymentService(repo repository.PaymentRepository) *DefaultPaymentService {
	return &DefaultPaymentService{repo: repo}
}

// ProcessPayment implements the Phase 2 synchronous transactional execution.
// It verifies idempotency:
// - Same key + same payload: returns original payment
// - Same key + different payload: returns ErrIdempotencyConflict (HTTP 409)
// - New key: inserts PENDING and immediately executes atomic transfer
func (s *DefaultPaymentService) ProcessPayment(
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
			// Idempotency key exists: fetch original payment
			existing, getErr := s.repo.GetByIdempotencyKey(ctx, idempotencyKey)
			if getErr != nil {
				return nil, fmt.Errorf("failed to retrieve conflicting payment: %w", getErr)
			}

			// Validate payload equality
			if !existing.MatchesPayload(fromID, toID, amount) {
				return nil, domain.ErrIdempotencyConflict
			}

			return existing, nil
		}
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	// In Phase 2, execute the transfer synchronously
	processed, err := s.repo.ExecuteTransfer(ctx, newPayment.ID)
	if err != nil {
		return nil, fmt.Errorf("transfer execution failed: %w", err)
	}

	return processed, nil
}

func (s *DefaultPaymentService) GetPayment(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	return s.repo.GetByID(ctx, id)
}
