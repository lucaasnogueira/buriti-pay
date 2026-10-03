package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/lucaasnogueira/buriti-pay/internal/domain"
)

func TestPayment_StateMachine(t *testing.T) {
	fromID := uuid.New()
	toID := uuid.New()
	key := "test-idem-key-1"
	amount := int64(2500)

	t.Run("valid lifecycle: PENDING -> PROCESSING -> CONFIRMED", func(t *testing.T) {
		p, err := domain.NewPayment(key, fromID, toID, amount)
		if err != nil {
			t.Fatalf("failed to create payment: %v", err)
		}

		if p.Status != domain.StatusPending {
			t.Fatalf("expected PENDING, got %s", p.Status)
		}

		if err := p.TransitionTo(domain.StatusProcessing, nil); err != nil {
			t.Fatalf("failed transition to PROCESSING: %v", err)
		}

		if err := p.TransitionTo(domain.StatusConfirmed, nil); err != nil {
			t.Fatalf("failed transition to CONFIRMED: %v", err)
		}

		// CONFIRMED is terminal
		err = p.TransitionTo(domain.StatusFailed, nil)
		if err == nil {
			t.Fatalf("expected error transitioning from terminal CONFIRMED, got nil")
		}
	})

	t.Run("valid lifecycle: PENDING -> PROCESSING -> FAILED", func(t *testing.T) {
		p, err := domain.NewPayment(key, fromID, toID, amount)
		if err != nil {
			t.Fatalf("failed to create payment: %v", err)
		}

		if err := p.TransitionTo(domain.StatusProcessing, nil); err != nil {
			t.Fatalf("failed transition to PROCESSING: %v", err)
		}

		failReason := "insufficient funds"
		if err := p.TransitionTo(domain.StatusFailed, &failReason); err != nil {
			t.Fatalf("failed transition to FAILED: %v", err)
		}

		if p.FailureReason == nil || *p.FailureReason != failReason {
			t.Errorf("expected failure reason %s", failReason)
		}

		// FAILED is terminal
		err = p.TransitionTo(domain.StatusPending, nil)
		if err == nil {
			t.Fatalf("expected error transitioning from terminal FAILED, got nil")
		}
	})

	t.Run("transient retry: PENDING -> PROCESSING -> PENDING", func(t *testing.T) {
		p, err := domain.NewPayment(key, fromID, toID, amount)
		if err != nil {
			t.Fatalf("failed to create payment: %v", err)
		}

		_ = p.TransitionTo(domain.StatusProcessing, nil)
		if err := p.TransitionTo(domain.StatusPending, nil); err != nil {
			t.Fatalf("failed transient retry to PENDING: %v", err)
		}

		if p.Status != domain.StatusPending {
			t.Errorf("expected status PENDING, got %s", p.Status)
		}
	})

	t.Run("invalid direct transition: PENDING -> CONFIRMED", func(t *testing.T) {
		p, _ := domain.NewPayment(key, fromID, toID, amount)
		if err := p.TransitionTo(domain.StatusConfirmed, nil); err == nil {
			t.Fatalf("expected error for illegal direct jump from PENDING to CONFIRMED")
		}
	})
}

func TestPayment_ValidationAndIdempotency(t *testing.T) {
	fromID := uuid.New()
	toID := uuid.New()

	t.Run("reject same account transfer", func(t *testing.T) {
		_, err := domain.NewPayment("k1", fromID, fromID, 100)
		if err != domain.ErrSameAccountTransfer {
			t.Fatalf("expected ErrSameAccountTransfer, got %v", err)
		}
	})

	t.Run("reject non-positive amount", func(t *testing.T) {
		_, err := domain.NewPayment("k1", fromID, toID, 0)
		if err != domain.ErrInvalidAmount {
			t.Fatalf("expected ErrInvalidAmount, got %v", err)
		}
	})

	t.Run("reject empty idempotency key", func(t *testing.T) {
		_, err := domain.NewPayment("", fromID, toID, 500)
		if err != domain.ErrIdempotencyKeyRequired {
			t.Fatalf("expected ErrIdempotencyKeyRequired, got %v", err)
		}
	})

	t.Run("matches payload comparison", func(t *testing.T) {
		p, _ := domain.NewPayment("k1", fromID, toID, 500)
		if !p.MatchesPayload(fromID, toID, 500) {
			t.Errorf("expected matchesPayload to return true for identical inputs")
		}
		if p.MatchesPayload(fromID, toID, 600) {
			t.Errorf("expected matchesPayload to return false for different amount")
		}
	})
}
