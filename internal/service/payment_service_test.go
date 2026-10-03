package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lucaasnogueira/buriti-pay/internal/domain"
	"github.com/lucaasnogueira/buriti-pay/internal/repository"
	"github.com/lucaasnogueira/buriti-pay/internal/service"
	"github.com/lucaasnogueira/buriti-pay/internal/worker"
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

func (m *mockPaymentRepo) FindStalePayments(ctx context.Context, staleAfter time.Duration, limit int) ([]uuid.UUID, error) {
	return nil, nil
}

type testEnqueuer struct {
	enqueued []worker.Job
}

func (e *testEnqueuer) Enqueue(job worker.Job) error {
	e.enqueued = append(e.enqueued, job)
	return nil
}

func TestPaymentService_AsyncIngestionAndIdempotency(t *testing.T) {
	repo := newMockPaymentRepo()
	enqueuer := &testEnqueuer{}
	svc := service.NewPaymentService(repo, enqueuer)
	ctx := context.Background()

	fromID := uuid.New()
	toID := uuid.New()
	key := "idem-key-phase3"
	amount := int64(3000)

	t.Run("creates payment with PENDING status and enqueues job", func(t *testing.T) {
		p, err := svc.CreatePaymentAsync(ctx, key, fromID, toID, amount)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Status != domain.StatusPending {
			t.Errorf("expected status PENDING, got %s", p.Status)
		}
		if len(enqueuer.enqueued) != 1 {
			t.Fatalf("expected 1 enqueued job, got %d", len(enqueuer.enqueued))
		}
	})

	t.Run("returns original payment for duplicate idempotency key without duplicate enqueue", func(t *testing.T) {
		p, err := svc.CreatePaymentAsync(ctx, key, fromID, toID, amount)
		if err != nil {
			t.Fatalf("expected idempotent success, got %v", err)
		}
		if p.IdempotencyKey != key {
			t.Errorf("expected key %s, got %s", key, p.IdempotencyKey)
		}
		// Enqueued count remains 1
		if len(enqueuer.enqueued) != 1 {
			t.Fatalf("expected enqueued count to stay 1, got %d", len(enqueuer.enqueued))
		}
	})

	t.Run("returns ErrIdempotencyConflict when payload differs", func(t *testing.T) {
		_, err := svc.CreatePaymentAsync(ctx, key, fromID, toID, 9999)
		if err != domain.ErrIdempotencyConflict {
			t.Fatalf("expected ErrIdempotencyConflict, got %v", err)
		}
	})

	t.Run("worker Process executes transfer to confirmed", func(t *testing.T) {
		err := svc.Process(ctx, enqueuer.enqueued[0])
		if err != nil {
			t.Fatalf("process failed: %v", err)
		}
		saved, _ := repo.GetByID(ctx, enqueuer.enqueued[0].PaymentID)
		if saved.Status != domain.StatusConfirmed {
			t.Errorf("expected CONFIRMED status, got %s", saved.Status)
		}
	})
}
