package service_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lucaasnogueira/buriti-pay/internal/domain"
	"github.com/lucaasnogueira/buriti-pay/internal/lock"
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

func (m *mockPaymentRepo) GetPendingOutboxEvents(ctx context.Context, limit int) ([]*domain.OutboxEvent, error) {
	return nil, nil
}

func (m *mockPaymentRepo) MarkOutboxEventPublished(ctx context.Context, id int64) error {
	return nil
}

type testEnqueuer struct {
	enqueued []worker.Job
}

func (e *testEnqueuer) Enqueue(job worker.Job) error {
	e.enqueued = append(e.enqueued, job)
	return nil
}

type mockLocker struct {
	acquiredKeys []string
	releasedKeys []string
}

func (m *mockLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (*lock.LockHandle, error) {
	m.acquiredKeys = append(m.acquiredKeys, key)
	return &lock.LockHandle{Key: key, Token: "token-123"}, nil
}

func (m *mockLocker) Release(ctx context.Context, handle *lock.LockHandle) error {
	if handle != nil {
		m.releasedKeys = append(m.releasedKeys, handle.Key)
	}
	return nil
}

func (m *mockLocker) Extend(ctx context.Context, handle *lock.LockHandle, ttl time.Duration) error {
	return nil
}

func TestPaymentService_DistributedLockAndOrdering(t *testing.T) {
	repo := newMockPaymentRepo()
	enqueuer := &testEnqueuer{}
	locker := &mockLocker{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc := service.NewPaymentService(repo, locker, enqueuer, 5*time.Second, logger)
	ctx := context.Background()

	id1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	id2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	// Create payment where fromID > toID (id2 -> id1)
	p, err := svc.CreatePaymentAsync(ctx, "key-order-test", id2, id1, 1000)
	if err != nil {
		t.Fatalf("failed to create payment: %v", err)
	}

	// Process job
	err = svc.Process(ctx, worker.Job{PaymentID: p.ID})
	if err != nil {
		t.Fatalf("failed to process job: %v", err)
	}

	// Verify locks were acquired deterministically in ascending UUID order (id1 first, then id2)
	expectedFirstKey := "lock:account:11111111-1111-1111-1111-111111111111"
	expectedSecondKey := "lock:account:22222222-2222-2222-2222-222222222222"

	if len(locker.acquiredKeys) != 2 {
		t.Fatalf("expected 2 acquired locks, got %d", len(locker.acquiredKeys))
	}
	if locker.acquiredKeys[0] != expectedFirstKey {
		t.Errorf("expected first lock to be %s, got %s", expectedFirstKey, locker.acquiredKeys[0])
	}
	if locker.acquiredKeys[1] != expectedSecondKey {
		t.Errorf("expected second lock to be %s, got %s", expectedSecondKey, locker.acquiredKeys[1])
	}

	// Verify all acquired locks were safely released
	if len(locker.releasedKeys) != 2 {
		t.Fatalf("expected 2 released locks, got %d", len(locker.releasedKeys))
	}
}
