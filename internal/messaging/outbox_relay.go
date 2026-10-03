package messaging

import (
	"context"
	"log/slog"
	"time"

	"github.com/lucaasnogueira/buriti-pay/internal/domain"
)

type OutboxStore interface {
	GetPendingOutboxEvents(ctx context.Context, limit int) ([]*domain.OutboxEvent, error)
	MarkOutboxEventPublished(ctx context.Context, id int64) error
}

type OutboxRelay struct {
	logger    *slog.Logger
	store     OutboxStore
	publisher EventPublisher
	interval  time.Duration
	batchSize int
	stopChan  chan struct{}
}

func NewOutboxRelay(
	logger *slog.Logger,
	store OutboxStore,
	publisher EventPublisher,
	interval time.Duration,
	batchSize int,
) *OutboxRelay {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	if batchSize <= 0 {
		batchSize = 50
	}

	return &OutboxRelay{
		logger:    logger,
		store:     store,
		publisher: publisher,
		interval:  interval,
		batchSize: batchSize,
		stopChan:  make(chan struct{}),
	}
}

func (r *OutboxRelay) Start(ctx context.Context) {
	go func() {
		r.logger.Info("Outbox Relay started", "interval", r.interval, "batch_size", r.batchSize)
		r.relay(ctx)

		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		for {
			select {
			case <-r.stopChan:
				r.logger.Info("Outbox Relay stopped")
				return
			case <-ctx.Done():
				r.logger.Info("Outbox Relay context cancelled, stopping")
				return
			case <-ticker.C:
				r.relay(ctx)
			}
		}
	}()
}

func (r *OutboxRelay) Stop() {
	close(r.stopChan)
}

func (r *OutboxRelay) relay(ctx context.Context) {
	events, err := r.store.GetPendingOutboxEvents(ctx, r.batchSize)
	if err != nil {
		r.logger.Error("Outbox Relay failed to fetch pending events", "error", err)
		return
	}

	for _, event := range events {
		if err := r.publisher.Publish(ctx, event.EventType, event.Payload); err != nil {
			r.logger.Warn("Outbox Relay failed to publish event to broker (will retry)",
				"event_id", event.ID,
				"aggregate_id", event.AggregateID.String(),
				"event_type", event.EventType,
				"error", err,
			)
			// Break batch on broker error to maintain sequential delivery
			return
		}

		if err := r.store.MarkOutboxEventPublished(ctx, event.ID); err != nil {
			r.logger.Error("Outbox Relay failed to mark event as published in database",
				"event_id", event.ID,
				"error", err,
			)
		} else {
			r.logger.Debug("Outbox Relay successfully published and marked event",
				"event_id", event.ID,
				"event_type", event.EventType,
			)
		}
	}
}
