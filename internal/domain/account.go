package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidAmount       = errors.New("amount must be greater than zero")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrAccountNotFound     = errors.New("account not found")
	ErrOwnerRequired       = errors.New("owner is required")
)

// Account represents a customer balance in the system.
// Balances are strictly represented as integer cents (ADR-0002).
type Account struct {
	ID        uuid.UUID `json:"id"`
	Owner     string    `json:"owner"`
	Balance   int64     `json:"balance"` // cents
	Version   int64     `json:"version"` // optimistic lock version
	CreatedAt time.Time `json:"created_at"`
}

func NewAccount(owner string, initialBalance int64) (*Account, error) {
	if owner == "" {
		return nil, ErrOwnerRequired
	}
	if initialBalance < 0 {
		return nil, errors.New("initial balance cannot be negative")
	}

	return &Account{
		ID:        uuid.New(),
		Owner:     owner,
		Balance:   initialBalance,
		Version:   0,
		CreatedAt: time.Now().UTC(),
	}, nil
}
