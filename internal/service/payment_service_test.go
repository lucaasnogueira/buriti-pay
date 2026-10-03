package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/lucaasnogueira/buriti-pay/internal/domain"
	"github.com/lucaasnogueira/buriti-pay/internal/repository"
	"github.com/lucaasnogueira/buriti-pay/internal/service"
)

type mockPaymentRepo struct {
	paymentsByKey map[string]*domain.Payment
	paymentsByID  map[uuid.UUID]*domain.Payment
}

func newMockPaymentRepo() *mockPaymentRepo {
	return &mockPaymentRepo{
		paymentsByKey: make(map[string]*domain.Payment),
		paymentsByID:  make(map[uuid.UUID]*domain.Payment),
	}
}

func (m *mockPaymentRepo) Create(ctx context.Context, p *domain.Payment) error {
	if _, exists := m.paymentsByKey[p.IdempotencyKey]; exists {
		return repository.ErrUniqueViolation
	}
	m.paymentsByKey[p.IdempotencyKey] = p
	m.paymentsByID[p.ID] = p
	return nil
}

func (m *mockPaymentRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	p, ok := m.paymentsByID[id]
	if !ok {
		return nil, domain.ErrPaymentNotFound
	}
	return p, nil
}

func (m *mockPaymentRepo) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	p, ok := m.paymentsByKey[key]
	if !ok {
		return nil, domain.ErrPaymentNotFound
	}
	return p, nil
}

func (m *mockPaymentRepo) ExecuteTransfer(ctx context.Context, paymentID uuid.UUID) (*domain.Payment, error) {
	p, ok := m.paymentsByID[paymentID]
	if !ok {
		return nil, domain.ErrPaymentNotFound
	}
	_ = p.TransitionTo(domain.StatusProcessing, nil)
	_ = p.TransitionTo(domain.StatusConfirmed, nil)
	return p, nil
}

func TestPaymentService_Idempotency(t *testing.T) {
	repo := newMockPaymentRepo()
	svc := service.NewPaymentService(repo)
	ctx := context.Background()

	fromID := uuid.New()
	toID := uuid.New()
	key := "idem-key-abc"
	amount := int64(1000)

	t.Run("first execution succeeds and creates payment", func(t *testing.T) {
		p, err := svc.ProcessPayment(ctx, key, fromID, toID, amount)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Status != domain.StatusConfirmed {
			t.Errorf("expected status CONFIRMED, got %s", p.Status)
		}
	})

	t.Run("second execution with same key and same payload returns original without error", func(t *testing.T) {
		p, err := svc.ProcessPayment(ctx, key, fromID, toID, amount)
		if err != nil {
			t.Fatalf("expected idempotent response, got error: %v", err)
		}
		if p.IdempotencyKey != key {
			t.Errorf("expected original payment key %s, got %s", key, p.IdempotencyKey)
		}
	})

	t.Run("execution with same key and different payload returns ErrIdempotencyConflict", func(t *testing.T) {
		differentAmount := int64(5000)
		_, err := svc.ProcessPayment(ctx, key, fromID, toID, differentAmount)
		if err != domain.ErrIdempotencyConflict {
			t.Fatalf("expected ErrIdempotencyConflict, got %v", err)
		}
	})
}
