package worker_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lucaasnogueira/buriti-pay/internal/worker"
)

type dummyHandler struct {
	processedCount int64
	shouldPanic    bool
}

func (h *dummyHandler) Process(ctx context.Context, job worker.Job) error {
	if h.shouldPanic {
		panic("simulated worker failure")
	}
	atomic.AddInt64(&h.processedCount, 1)
	return nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestWorkerPool_ConcurrencyAndRecovery(t *testing.T) {
	t.Run("concurrent processing of 1000 jobs", func(t *testing.T) {
		handler := &dummyHandler{}
		pool := worker.NewPool(testLogger(), handler, 8, 1000, 2*time.Second)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool.Start(ctx)

		const totalJobs = 1000
		for i := 0; i < totalJobs; i++ {
			err := pool.Enqueue(worker.Job{PaymentID: uuid.New()})
			if err != nil {
				t.Fatalf("failed to enqueue job %d: %v", i, err)
			}
		}

		// Allow workers to drain
		err := pool.Stop(3 * time.Second)
		if err != nil {
			t.Fatalf("error stopping pool: %v", err)
		}

		processed := atomic.LoadInt64(&handler.processedCount)
		if processed != totalJobs {
			t.Errorf("expected %d processed jobs, got %d", totalJobs, processed)
		}
	})

	t.Run("backpressure returns ErrQueueFull when buffer is saturated", func(t *testing.T) {
		handler := &dummyHandler{}
		// Pool with 1 buffer slot and zero workers running
		pool := worker.NewPool(testLogger(), handler, 1, 1, 1*time.Second)

		// First job fills the buffer
		err1 := pool.Enqueue(worker.Job{PaymentID: uuid.New()})
		if err1 != nil {
			t.Fatalf("expected first job to succeed, got %v", err1)
		}

		// Second job must be rejected by backpressure
		err2 := pool.Enqueue(worker.Job{PaymentID: uuid.New()})
		if err2 != worker.ErrQueueFull {
			t.Fatalf("expected ErrQueueFull, got %v", err2)
		}
	})

	t.Run("worker panic recovery does not crash pool", func(t *testing.T) {
		handler := &dummyHandler{shouldPanic: true}
		pool := worker.NewPool(testLogger(), handler, 2, 10, 1*time.Second)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool.Start(ctx)

		// Send job that triggers a panic
		_ = pool.Enqueue(worker.Job{PaymentID: uuid.New()})

		time.Sleep(100 * time.Millisecond)

		// Pool should remain alive and stop cleanly
		err := pool.Stop(1 * time.Second)
		if err != nil {
			t.Fatalf("expected clean stop after panic recovery, got %v", err)
		}
	})
}

type mockStaleFinder struct {
	staleIDs []uuid.UUID
}

func (m *mockStaleFinder) FindStalePayments(ctx context.Context, staleAfter time.Duration, limit int) ([]uuid.UUID, error) {
	return m.staleIDs, nil
}

type mockEnqueuer struct {
	mu     sync.Mutex
	queued []worker.Job
}

func (m *mockEnqueuer) Enqueue(job worker.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queued = append(m.queued, job)
	return nil
}

func TestReaper_ReenqueuesStalePayments(t *testing.T) {
	staleID := uuid.New()
	finder := &mockStaleFinder{staleIDs: []uuid.UUID{staleID}}
	enqueuer := &mockEnqueuer{}

	reaper := worker.NewReaper(testLogger(), finder, enqueuer, 50*time.Millisecond, 1*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reaper.Start(ctx)

	// Wait for immediate startup scan or first tick
	time.Sleep(100 * time.Millisecond)
	reaper.Stop()

	enqueuer.mu.Lock()
	count := len(enqueuer.queued)
	enqueuer.mu.Unlock()

	if count == 0 {
		t.Fatalf("expected reaper to re-enqueue stale payment, got 0")
	}
}
