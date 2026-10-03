package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidPaymentStatusTransition = errors.New("invalid payment status transition")
	ErrSameAccountTransfer            = errors.New("source and destination accounts must be different")
	ErrIdempotencyKeyRequired         = errors.New("idempotency key is required")
	ErrIdempotencyConflict           = errors.New("idempotency key reused with different request payload")
	ErrPaymentNotFound                = errors.New("payment not found")
)

type PaymentStatus string

const (
	StatusPending    PaymentStatus = "PENDING"
	StatusProcessing PaymentStatus = "PROCESSING"
	StatusConfirmed  PaymentStatus = "CONFIRMED"
	StatusFailed     PaymentStatus = "FAILED"
)

func (s PaymentStatus) IsValid() bool {
	switch s {
	case StatusPending, StatusProcessing, StatusConfirmed, StatusFailed:
		return true
	default:
		return false
	}
}

type Payment struct {
	ID             uuid.UUID     `json:"id"`
	IdempotencyKey string        `json:"idempotency_key"`
	FromAccountID  uuid.UUID     `json:"from_account_id"`
	ToAccountID    uuid.UUID     `json:"to_account_id"`
	Amount         int64         `json:"amount"` // in cents
	Status         PaymentStatus `json:"status"`
	FailureReason  *string       `json:"failure_reason,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

func NewPayment(idempotencyKey string, fromAccountID, toAccountID uuid.UUID, amount int64) (*Payment, error) {
	if idempotencyKey == "" {
		return nil, ErrIdempotencyKeyRequired
	}
	if fromAccountID == toAccountID {
		return nil, ErrSameAccountTransfer
	}
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}

	now := time.Now().UTC()
	return &Payment{
		ID:             uuid.New(),
		IdempotencyKey: idempotencyKey,
		FromAccountID:  fromAccountID,
		ToAccountID:    toAccountID,
		Amount:         amount,
		Status:         StatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// TransitionTo validates and performs the payment state machine transitions.
// Valid paths:
// (new)      -> PENDING
// PENDING    -> PROCESSING
// PROCESSING -> CONFIRMED (terminal)
// PROCESSING -> FAILED (terminal)
// PROCESSING -> PENDING (transient retry / timeout)
func (p *Payment) TransitionTo(target PaymentStatus, reason *string) error {
	switch p.Status {
	case StatusPending:
		if target == StatusProcessing {
			p.Status = target
			p.UpdatedAt = time.Now().UTC()
			return nil
		}
	case StatusProcessing:
		if target == StatusConfirmed {
			p.Status = target
			p.FailureReason = nil
			p.UpdatedAt = time.Now().UTC()
			return nil
		}
		if target == StatusFailed {
			p.Status = target
			p.FailureReason = reason
			p.UpdatedAt = time.Now().UTC()
			return nil
		}
		if target == StatusPending {
			// Back to pending for retry/re-enqueue
			p.Status = target
			p.UpdatedAt = time.Now().UTC()
			return nil
		}
	case StatusConfirmed, StatusFailed:
		return fmt.Errorf("%w: terminal state %s cannot be changed", ErrInvalidPaymentStatusTransition, p.Status)
	}

	return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidPaymentStatusTransition, p.Status, target)
}

// MatchesPayload checks if an existing payment has the exact same payload as a new request.
func (p *Payment) MatchesPayload(fromAccountID, toAccountID uuid.UUID, amount int64) bool {
	return p.FromAccountID == fromAccountID &&
		p.ToAccountID == toAccountID &&
		p.Amount == amount
}
