package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// LedgerEntry represents an immutable double-entry bookkeeping record.
// For every payment, sum(delta) == 0.
type LedgerEntry struct {
	ID        int64     `json:"id"`
	PaymentID uuid.UUID `json:"payment_id"`
	AccountID uuid.UUID `json:"account_id"`
	Delta     int64     `json:"delta"` // negative = debit, positive = credit
	CreatedAt time.Time `json:"created_at"`
}

// OutboxEvent represents an event staged inside the database transaction (ADR-0005).
type OutboxEvent struct {
	ID          int64           `json:"id"`
	AggregateID uuid.UUID       `json:"aggregate_id"`
	EventType   string          `json:"event_type"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"created_at"`
	PublishedAt *time.Time      `json:"published_at,omitempty"`
}

type PaymentConfirmedPayload struct {
	PaymentID     uuid.UUID `json:"payment_id"`
	FromAccountID uuid.UUID `json:"from_account_id"`
	ToAccountID   uuid.UUID `json:"to_account_id"`
	Amount        int64     `json:"amount"`
	Status        string    `json:"status"`
}

type PaymentFailedPayload struct {
	PaymentID     uuid.UUID `json:"payment_id"`
	FromAccountID uuid.UUID `json:"from_account_id"`
	ToAccountID   uuid.UUID `json:"to_account_id"`
	Amount        int64     `json:"amount"`
	Status        string    `json:"status"`
	Reason        string    `json:"reason"`
}
