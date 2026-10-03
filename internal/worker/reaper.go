package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

type StalePaymentFinder interface {
	FindStalePayments(ctx context.Context, staleAfter time.Duration, limit int) ([]uuid.UUID, error)
}

type Enqueuer interface {
	Enqueue(job Job) error
}

type Reaper struct {
	logger     *slog.Logger
	finder     StalePaymentFinder
	enqueuer   Enqueuer
	interval   time.Duration
	staleAfter time.Duration
	batchLimit int
	stopChan   chan struct{}
}

func NewReaper(
	logger *slog.Logger,
	finder StalePaymentFinder,
	enqueuer Enqueuer,
	interval time.Duration,
	staleAfter time.Duration,
) *Reaper {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if staleAfter <= 0 {
		staleAfter = 60 * time.Second
	}

	return &Reaper{
		logger:     logger,
		finder:     finder,
		enqueuer:   enqueuer,
		interval:   interval,
		staleAfter: staleAfter,
		batchLimit: 100,
		stopChan:   make(chan struct{}),
	}
}

func (r *Reaper) Start(ctx context.Context) {
	go func() {
		r.logger.Info("Reaper background process started", "interval", r.interval, "stale_after", r.staleAfter)
		// Run immediate scan at startup
		r.reap(ctx)

		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		for {
			select {
			case <-r.stopChan:
				r.logger.Info("Reaper stopped")
				return
			case <-ctx.Done():
				r.logger.Info("Reaper context cancelled, stopping")
				return
			case <-ticker.C:
				r.reap(ctx)
			}
		}
	}()
}

func (r *Reaper) Stop() {
	close(r.stopChan)
}

func (r *Reaper) reap(ctx context.Context) {
	staleIDs, err := r.finder.FindStalePayments(ctx, r.staleAfter, r.batchLimit)
	if err != nil {
		r.logger.Error("Reaper failed to query stale payments", "error", err)
		return
	}

	if len(staleIDs) == 0 {
		return
	}

	r.logger.Info("Reaper found stale payments to re-enqueue", "count", len(staleIDs))
	for _, id := range staleIDs {
		job := Job{PaymentID: id}
		if err := r.enqueuer.Enqueue(job); err != nil {
			r.logger.Warn("Reaper could not re-enqueue job (queue saturated)", "payment_id", id.String())
		} else {
			r.logger.Info("Reaper successfully re-enqueued payment", "payment_id", id.String())
		}
	}
}
