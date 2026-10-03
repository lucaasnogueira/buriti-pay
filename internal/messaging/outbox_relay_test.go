package messaging_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lucaasnogueira/buriti-pay/internal/domain"
	"github.com/lucaasnogueira/buriti-pay/internal/messaging"
)

type mockOutboxStore struct {
	mu        sync.Mutex
	events    []*domain.OutboxEvent
	published []int64
}

func (m *mockOutboxStore) GetPendingOutboxEvents(ctx context.Context, limit int) ([]*domain.OutboxEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var pending []*domain.OutboxEvent
	for _, e := range m.events {
		if e.PublishedAt == nil {
			pending = append(pending, e)
			if len(pending) >= limit {
				break
			}
		}
	}
	return pending, nil
}

func (m *mockOutboxStore) MarkOutboxEventPublished(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	for _, e := range m.events {
		if e.ID == id {
			e.PublishedAt = &now
			m.published = append(m.published, id)
			break
		}
	}
	return nil
}

type mockPublisher struct {
	mu        sync.Mutex
	published map[string][]byte
}

func (m *mockPublisher) Publish(ctx context.Context, routingKey string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published[routingKey] = body
	return nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestOutboxRelay_PublishesAndMarksPendingEvents(t *testing.T) {
	rawPayload, _ := json.Marshal(domain.PaymentConfirmedPayload{
		PaymentID:     uuid.New(),
		FromAccountID: uuid.New(),
		ToAccountID:   uuid.New(),
		Amount:        5000,
		Status:        "CONFIRMED",
	})

	store := &mockOutboxStore{
		events: []*domain.OutboxEvent{
			{
				ID:          1,
				AggregateID: uuid.New(),
				EventType:   messaging.PaymentConfirmedKey,
				Payload:     rawPayload,
				CreatedAt:   time.Now().UTC(),
			},
		},
	}
	pub := &mockPublisher{published: make(map[string][]byte)}

	relay := messaging.NewOutboxRelay(testLogger(), store, pub, 50*time.Millisecond, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	relay.Start(ctx)

	// Wait for processing
	time.Sleep(150 * time.Millisecond)
	relay.Stop()

	store.mu.Lock()
	publishedCount := len(store.published)
	store.mu.Unlock()

	if publishedCount != 1 {
		t.Fatalf("expected 1 event marked published, got %d", publishedCount)
	}

	pub.mu.Lock()
	_, found := pub.published[messaging.PaymentConfirmedKey]
	pub.mu.Unlock()

	if !found {
		t.Errorf("expected event published to routing key %s", messaging.PaymentConfirmedKey)
	}
}
